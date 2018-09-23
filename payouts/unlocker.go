package payouts

import (
	"fmt"
	"log"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/NotoriousPyro/open-metaverse-pool/rpc"
	"github.com/NotoriousPyro/open-metaverse-pool/storage"
	"github.com/NotoriousPyro/open-metaverse-pool/util"
)

type UnlockerConfig struct {
	Enabled			bool    `json:"enabled"`
	PoolFee			float64 `json:"poolFee"`
	Donate			bool    `json:"donate"`
	Depth			int64   `json:"depth"`
	ImmatureDepth	int64   `json:"immatureDepth"`
	KeepTxFees		bool    `json:"keepTxFees"`
	Interval		string  `json:"interval"`
	Daemon			string  `json:"daemon"`
	Timeout			string  `json:"timeout"`
	Account			string
	Password		string
	Address			string	`json:"address"`
	PoolFeeAddress	string	`json:"poolFeeAddress"`
}

const minDepth = 16

type BlockUnlocker struct {
	config		*UnlockerConfig
	backend		*storage.RedisClient
	rpc			*rpc.RPCClient
	halt		bool
	lastFail	error
}

func NewBlockUnlocker(cfg *UnlockerConfig, backend *storage.RedisClient) *BlockUnlocker {
	if cfg.Depth < minDepth*2 {
		log.Fatalf("Block maturity depth can't be < %v, your depth is %v", minDepth*2, cfg.Depth)
	}
	if cfg.ImmatureDepth < minDepth {
		log.Fatalf("Immature depth can't be < %v, your depth is %v", minDepth, cfg.ImmatureDepth)
	}
	u := &BlockUnlocker{config: cfg, backend: backend}
	if len(cfg.PoolFeeAddress) != 0 && !util.IsValidHexAddress(cfg.PoolFeeAddress) {
		log.Fatalln("Invalid poolFeeAddress", cfg.PoolFeeAddress)
	}
	if len(cfg.PoolFeeAddress) < 1 {
		log.Fatalln("poolFeeAddress not set in config", cfg.PoolFeeAddress)
	}
	u.rpc = rpc.NewRPCClient("BlockUnlocker", cfg.Daemon, cfg.Account, cfg.Password, cfg.Timeout)
	return u
}

func (u *BlockUnlocker) Start() {
	log.Println("Starting block unlocker")
	intv := util.MustParseDuration(u.config.Interval)
	timer := time.NewTimer(intv)
	log.Printf("Set block unlock interval to %v", intv)

	// Immediately unlock after start
	u.unlockPendingBlocks()
	u.unlockAndCreditMiners()
	timer.Reset(intv)

	go func() {
		for {
			select {
			case <-timer.C:
				u.unlockPendingBlocks()
				u.unlockAndCreditMiners()
				timer.Reset(intv)
			}
		}
	}()
}

type UnlockResult struct {
	maturedBlocks  []*storage.BlockData
	orphanedBlocks []*storage.BlockData
	orphans        int
	uncles         int
	blocks         int
}

func (u *BlockUnlocker) unlockCandidates(candidates []*storage.BlockData) (*UnlockResult, error) {
	result := &UnlockResult{}

	// Data row is: "height:nonce:powHash:mixDigest:timestamp:diff:totalShares"
	for _, candidate := range candidates {
		height := candidate.Height
		block, err := u.rpc.GetBlockByHeight(height)
		if err != nil {
			log.Printf("Error while retrieving block %v from node: %v", height, err)
			return nil, err
		}
		if block == nil {
			return nil, fmt.Errorf("Error while retrieving block %v from node, wrong node height", height)
		}
		
		if u.matchCandidate(block, candidate) {
			result.blocks++

			err = u.handleBlock(block, candidate)
			if err != nil {
				u.halt = true
				u.lastFail = err
				return nil, err
			}
			result.maturedBlocks = append(result.maturedBlocks, candidate)
			log.Printf("Mature block %v, hash: %v", candidate.Height, candidate.Hash)
		} else {
			result.orphans++
			candidate.Orphan = true
			result.orphanedBlocks = append(result.orphanedBlocks, candidate)
			log.Printf("Orphaned block %v:%v", candidate.RoundHeight, candidate.Nonce)
		}
	}
	return result, nil
}

func (u *BlockUnlocker) matchCandidate(block *rpc.GetBlockReply, candidate *storage.BlockData) bool {
	if len(block.Transactions) != 0 {
		if len (block.Transactions[0].Outputs) != 0 {
			if block.Transactions[0].Outputs[0].Address != u.config.Address {
				return false
			}
		}
	}
	
	if len(candidate.Hash) > 0 && !strings.EqualFold(candidate.Hash, block.Hash) {
		return false
	}
	nonce1, _ := strconv.ParseInt(block.Nonce, 10, 64)
	nonce2, _ := strconv.ParseInt(strings.Replace(candidate.Nonce, "0x", "", -1), 16, 64)
	return nonce1 == nonce2
}

func (u *BlockUnlocker) handleBlock(block *rpc.GetBlockReply, candidate *storage.BlockData) error {
	reward := big.NewInt(int64(300000000 * math.Pow(0.95, math.Floor(float64(block.Number*1.0)/500000))))
	extraTxReward, _ := u.getExtraRewardForTx(block.Number, reward)
	
	if u.config.KeepTxFees {
		candidate.ExtraReward = extraTxReward
	} else {
		reward.Add(reward, extraTxReward)
	}
	
	candidate.Height = int64(block.Number)
	candidate.Orphan = false
	candidate.Hash = block.Hash
	candidate.Reward = reward
	return nil
}

func (u *BlockUnlocker) unlockPendingBlocks() {
	if u.halt {
		log.Println("Unlocking suspended due to last critical error:", u.lastFail)
		return
	}

	current, err := u.rpc.GetPendingBlock()
	if err != nil {
		u.halt = true
		u.lastFail = err
		log.Printf("Unable to get current blockchain height from node: %v", err)
		return
	}

	candidates, err := u.backend.GetCandidates(int64(current.Number) - u.config.ImmatureDepth)
	if err != nil {
		u.halt = true
		u.lastFail = err
		log.Printf("Failed to get block candidates from backend: %v", err)
		return
	}
		
	if len(candidates) == 0 {
		log.Println("No block candidates to unlock")
		return
	}
	
	result, err := u.unlockCandidates(candidates)
	if err != nil {
		u.halt = true
		u.lastFail = err
		log.Printf("Failed to unlock blocks: %v", err)
		return
	}
	log.Printf("Immature %v blocks, %v uncles, %v orphans", result.blocks, result.uncles, result.orphans)

	err = u.backend.WritePendingOrphans(result.orphanedBlocks)
	if err != nil {
		u.halt = true
		u.lastFail = err
		log.Printf("Failed to insert orphaned blocks into backend: %v", err)
		return
	} else {
		log.Printf("Inserted %v orphaned blocks to backend", result.orphans)
	}

	totalRevenue := new(big.Rat)
	totalMinersProfit := new(big.Rat)
	totalPoolProfit := new(big.Rat)

	for _, block := range result.maturedBlocks {
		revenue, minersProfit, poolProfit, roundRewards, err := u.calculateRewards(block)
		if err != nil {
			u.halt = true
			u.lastFail = err
			log.Printf("Failed to calculate rewards for round %v: %v", block.RoundKey(), err)
			return
		}
		err = u.backend.WriteImmatureBlock(block, roundRewards)
		if err != nil {
			u.halt = true
			u.lastFail = err
			log.Printf("Failed to credit rewards for round %v: %v", block.RoundKey(), err)
			return
		}
		totalRevenue.Add(totalRevenue, revenue)
		totalMinersProfit.Add(totalMinersProfit, minersProfit)
		totalPoolProfit.Add(totalPoolProfit, poolProfit)

		logEntry := fmt.Sprintf(
			"IMMATURE %v: revenue %v, miners profit %v, pool profit: %v",
			block.RoundKey(),
			util.FormatRatReward(revenue),
			util.FormatRatReward(minersProfit),
			util.FormatRatReward(poolProfit),
		)
		entries := []string{logEntry}
		for login, reward := range roundRewards {
			entries = append(entries, fmt.Sprintf("\tREWARD %v: %v: %v Shannon", block.RoundKey(), login, reward))
		}
		log.Println(strings.Join(entries, "\n"))
	}

	log.Printf(
		"IMMATURE SESSION: revenue %v, miners profit %v, pool profit: %v",
		util.FormatRatReward(totalRevenue),
		util.FormatRatReward(totalMinersProfit),
		util.FormatRatReward(totalPoolProfit),
	)
}

func (u *BlockUnlocker) unlockAndCreditMiners() {
	if u.halt {
		log.Println("Unlocking suspended due to last critical error:", u.lastFail)
		return
	}
	
	current, err := u.rpc.GetPendingBlock()
	if err != nil {
		u.halt = true
		u.lastFail = err
		log.Printf("Unable to get current blockchain height from node: %v", err)
		return
	}
	
	immature, err := u.backend.GetImmatureBlocks(int64(current.Number) - u.config.Depth)
	if err != nil {
		u.halt = true
		u.lastFail = err
		log.Printf("Failed to get block candidates from backend: %v", err)
		return
	}

	if len(immature) == 0 {
		log.Println("No immature blocks to credit miners")
		return
	}

	result, err := u.unlockCandidates(immature)
	if err != nil {
		u.halt = true
		u.lastFail = err
		log.Printf("Failed to unlock blocks: %v", err)
		return
	}
	log.Printf("Unlocked %v blocks, %v uncles, %v orphans", result.blocks, result.uncles, result.orphans)

	for _, block := range result.orphanedBlocks {
		err = u.backend.WriteOrphan(block)
		if err != nil {
			u.halt = true
			u.lastFail = err
			log.Printf("Failed to insert orphaned block into backend: %v", err)
			return
		}
	}
	log.Printf("Inserted %v orphaned blocks to backend", result.orphans)

	totalRevenue := new(big.Rat)
	totalMinersProfit := new(big.Rat)
	totalPoolProfit := new(big.Rat)

	for _, block := range result.maturedBlocks {
		revenue, minersProfit, poolProfit, roundRewards, err := u.calculateRewards(block)
		if err != nil {
			u.halt = true
			u.lastFail = err
			log.Printf("Failed to calculate rewards for round %v: %v", block.RoundKey(), err)
			return
		}
		err = u.backend.WriteMaturedBlock(block, roundRewards)
		if err != nil {
			u.halt = true
			u.lastFail = err
			log.Printf("Failed to credit rewards for round %v: %v", block.RoundKey(), err)
			return
		}
		totalRevenue.Add(totalRevenue, revenue)
		totalMinersProfit.Add(totalMinersProfit, minersProfit)
		totalPoolProfit.Add(totalPoolProfit, poolProfit)

		logEntry := fmt.Sprintf(
			"MATURED %v: revenue %v, miners profit %v, pool profit: %v",
			block.RoundKey(),
			util.FormatRatReward(revenue),
			util.FormatRatReward(minersProfit),
			util.FormatRatReward(poolProfit),
		)
		entries := []string{logEntry}
		for login, reward := range roundRewards {
			entries = append(entries, fmt.Sprintf("\tREWARD %v: %v: %v Shannon", block.RoundKey(), login, reward))
		}
		log.Println(strings.Join(entries, "\n"))
	}

	log.Printf(
		"MATURE SESSION: revenue %v, miners profit %v, pool profit: %v",
		util.FormatRatReward(totalRevenue),
		util.FormatRatReward(totalMinersProfit),
		util.FormatRatReward(totalPoolProfit),
	)
}

func (u *BlockUnlocker) calculateRewards(block *storage.BlockData) (*big.Rat, *big.Rat, *big.Rat, map[string]int64, error) {
	revenue := new(big.Rat).SetInt(block.Reward)
	minersProfit, poolProfit := chargeFee(revenue, u.config.PoolFee)

	shares, err := u.backend.GetRoundShares(block.RoundHeight, block.Nonce)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	rewards := calculateRewardsForShares(shares, block.TotalShares, minersProfit)

	if block.ExtraReward != nil {
		extraReward := new(big.Rat).SetInt(block.ExtraReward)
		poolProfit.Add(poolProfit, extraReward)
		revenue.Add(revenue, extraReward)
	}

	if len(u.config.PoolFeeAddress) != 0 {
		address := u.config.PoolFeeAddress
		fee, _ := strconv.ParseInt(poolProfit.FloatString(0), 10, 64)
		rewards[address] += fee
	}

	return revenue, minersProfit, poolProfit, rewards, nil
}

func calculateRewardsForShares(shares map[string]int64, total int64, reward *big.Rat) map[string]int64 {
	rewards := make(map[string]int64)

	for login, n := range shares {
		if util.IsValidHexAddress(login) {
			percent := big.NewRat(n, total)
			workerReward := new(big.Rat).Mul(reward, percent)
			fee, _ := strconv.ParseInt(workerReward.FloatString(0), 10, 64)
			rewards[login] += fee
		}
	}
	return rewards
}

// Returns new value after fee deduction and fee value.
func chargeFee(value *big.Rat, fee float64) (*big.Rat, *big.Rat) {
	feePercent := new(big.Rat).SetFloat64(fee / 100)
	feeValue := new(big.Rat).Mul(value, feePercent)
	return new(big.Rat).Sub(value, feeValue), feeValue
}

func (u *BlockUnlocker) getExtraRewardForTx(height uint64, reward *big.Int) (*big.Int, error) {
	BlockTxs, err := u.rpc.GetBlockTxs(height)
	if err != nil {
		log.Printf("Error retrieving BlockTxs for height %v", height)
		return nil, err
	}
	
	blockValue := big.NewInt(BlockTxs.Transactions[0].Outputs[0].Value)
	if err != nil {
		return nil, err
	}
	
	return new(big.Int).Sub(blockValue, reward), nil
}
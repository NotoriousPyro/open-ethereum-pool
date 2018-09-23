package proxy

import (
    "log"
    "math/big"
    "sync"

    "github.com/ethereum/go-ethereum/common"
    "github.com/NotoriousPyro/open-metaverse-pool/rpc"
)

type BlockTemplate struct {
    sync.RWMutex
    Header                    string
    Seed                      string
    Target                    string
    Difficulty                *big.Int
    Height                    uint64
    GetPendingBlockCache      *rpc.GetBlockReply
    nonces                    map[string]bool
}

type Block struct {
    difficulty                *big.Int
    hashNoNonce               common.Hash
    nonce                     uint64
    mixDigest                 common.Hash
    number                    uint64
}

func (b Block) Difficulty() *big.Int     { return b.difficulty }
func (b Block) HashNoNonce() common.Hash { return b.hashNoNonce }
func (b Block) Nonce() uint64            { return b.nonce }
func (b Block) MixDigest() common.Hash   { return b.mixDigest }
func (b Block) NumberU64() uint64        { return b.number }

func (s *ProxyServer) fetchBlockTemplate() {
    rpc := s.rpc()
    t := s.currentBlockTemplate()
    
    pendingReply, height, diff, err := s.fetchPendingBlock()
    if err != nil {
        log.Printf("Error while refreshing pending block on %s: %s", rpc.Name, err)
        return
    }
    
    reply, err := rpc.GetWork()
    if err != nil {
        log.Printf("Error while refreshing block template on %s: %s", rpc.Name, err)
        return
    }
    
    if t != nil && t.Header == reply[0] {
        return
    }
    
    newTemplate := BlockTemplate{
        Header:                    reply[0],
        Seed:                    reply[1],
        Target:                    reply[2],
        Height:                    height,
        Difficulty:                diff,
        GetPendingBlockCache:    pendingReply,
    }
    
    s.blockTemplate.Store(&newTemplate)
    log.Printf("New block to mine on %s at height %d / %s", rpc.Name, height, reply[0])
    
    for i, setting := range s.config.Proxy.Stratum {
        if setting.Enabled {
            go s.broadcastNewJobs(i)
        }
     }
}

func (s *ProxyServer) fetchPendingBlock() (*rpc.GetBlockReply, uint64, *big.Int, error) {
    rpc := s.rpc()
    reply, err := rpc.GetPendingBlock()
    if err != nil {
        log.Printf("Error while refreshing pending block on %s: %s", rpc.Name, err)
        return nil, 0, nil, err
    }
    
    blockDiff, success := new(big.Int).SetString(reply.Difficulty, 10)
    if !success {
        log.Println("Can't parse pending block difficulty")
        return nil, 0, nil, err
    }
    
    return reply, reply.Number, blockDiff, nil
}

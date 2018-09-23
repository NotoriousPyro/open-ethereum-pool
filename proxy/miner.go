package proxy

import (
    "log"
    "math/big"
    "strconv"
    "strings"

    "github.com/ethereum/ethash"
    "github.com/ethereum/go-ethereum/common"
)

var hasher = ethash.New()

// returns exist, valid, stale as boolean
func (s *ProxyServer) processShare(login, id, ip string, t *BlockTemplate, params []string, s_id int) (bool, bool, bool) {
    stratumConfig := s.config.Proxy.Stratum[s_id]
    nonceHex := params[0]
    hashNoNonce := params[1]
    mixDigest := params[2]
    nonce, _ := strconv.ParseUint(strings.Replace(nonceHex, "0x", "", -1), 16, 64)
    shareDiff := stratumConfig.Difficulty
    
    if !strings.EqualFold(t.Header, hashNoNonce) {
        // Stale Share
        return false, false, true
    }
    
    share := Block{
        number:      t.Height,
        hashNoNonce: common.HexToHash(hashNoNonce),
        difficulty:  big.NewInt(shareDiff),
        nonce:       nonce,
        mixDigest:   common.HexToHash(mixDigest),
    }
    
    block := Block{
        number:      t.Height,
        hashNoNonce: common.HexToHash(hashNoNonce),
        difficulty:  t.Difficulty,
        nonce:       nonce,
        mixDigest:   common.HexToHash(mixDigest),
    }
    
    if !hasher.Verify(share) {
        // Invalid Share
        return false, false, false
    }
    
    if hasher.Verify(block) {
        ok, err := s.rpc().SubmitWork(params)
        if err != nil {
            log.Printf("Block submission failure at height %v for %v: %v", t.Height, t.Header, err)
        } else if !ok {
            log.Printf("Block rejected at height %v for %v", t.Height, t.Header)
            // Rejected Block
            return false, false, false
        } else {
            s.fetchBlockTemplate()
            exist, err := s.backend.WriteBlock(login, id, params, shareDiff, t.Difficulty.Int64(), t.Height, s.hashrateExpiration)
            if exist {
                // Duplicate Block
                return true, true, false
            }
            if err != nil {
                log.Println("Failed to insert block candidate into backend:", err)
            } else {
                // Valid Block
                log.Printf("Inserted block %v to backend", t.Height)
            }
            log.Printf("Block found by miner %v@%v at height %d", login, ip, t.Height)
        }
    } else {
        exist, err := s.backend.WriteShare(login, id, params, shareDiff, t.Height, s.hashrateExpiration)
        if exist {
            // Duplicate Share
            return true, true, false
        }
        if err != nil {
            log.Println("Failed to insert share data into backend:", err)
        }
    }
    // Valid Share
    return false, true, false
}

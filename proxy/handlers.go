package proxy

import (
	"log"
	"regexp"
	
	"github.com/NotoriousPyro/open-metaverse-pool/rpc"
	"github.com/NotoriousPyro/open-metaverse-pool/util"
)

var (
	noncePattern = regexp.MustCompile("^0x[0-9a-f]{16}$")
	hashPattern = regexp.MustCompile("^0x[0-9a-f]{64}$")
	workerPattern = regexp.MustCompile("^[0-9a-zA-Z-_]{1,8}$")
)

func (s *ProxyServer) handleLoginRPC(cs *Session, params []string, id string) (bool, *ErrorReply) {
	if len(params) == 0 {
		return false, &ErrorReply{Code: -1, Message: "Invalid params"}
	}
	
	login := params[0]
	
	if !util.IsValidHexAddress(login) {
		return false, &ErrorReply{Code: -1, Message: "Invalid login format."}
		s.policy.ApplyMalformedPolicy(cs.ip)
	}
	
	address, err := s.rpc().ValidateAddress(login)
	
	if !address.Valid() || err != nil || address == nil {
		return false, &ErrorReply{Code: 0, Message: "Invalid login."}
		s.policy.ApplyMalformedPolicy(cs.ip)
	}
	
	if !s.policy.ApplyLoginPolicy(login, cs.ip) {
		return false, &ErrorReply{Code: -1, Message: "You are blacklisted"}
	}
	
	cs.login = login
	s.registerSession(cs)
	
	stratumConfig := s.config.Proxy.Stratum[cs.s_id]
	
	log.Printf("Stratum miner connected on %s from %s : %s", stratumConfig.Name, cs.ip, login)
	
	return true, nil
}

func (s *ProxyServer) handleGetWorkRPC(cs *Session) ([]string, *ErrorReply) {
	t := s.currentBlockTemplate()
	if t == nil || len(t.Header) == 0 || s.isSick() {
		return nil, &ErrorReply{Code: 0, Message: "Work not ready"}
	}
	diff := s.stratum[cs.s_id].diff
	return []string{t.Header, t.Seed, diff}, nil
}

func (s *ProxyServer) handleTCPSubmitRPC(cs *Session, id string, params []string) (bool, *ErrorReply) {
	stratum := s.stratum[cs.s_id]
	stratum.sessionsMu.RLock()
	_, ok := stratum.sessions[cs]
	stratum.sessionsMu.RUnlock()

	if !ok {
		return false, &ErrorReply{Code: 25, Message: "Not subscribed"}
	}
	return s.handleSubmitRPC(cs, cs.login, id, params)
}

func (s *ProxyServer) handleSubmitRPC(cs *Session, login, id string, params []string) (bool, *ErrorReply) {
	stratumConfig := s.config.Proxy.Stratum[cs.s_id]
	if !workerPattern.MatchString(id) {
		id = "0"
	}
	if len(params) != 3 {
		s.policy.ApplyMalformedPolicy(cs.ip)
		log.Printf("Malformed params on %s from %s : %s %v", stratumConfig.Name, cs.ip, login, params)
		return false, &ErrorReply{Code: -1, Message: "Invalid params"}
	}

	if !noncePattern.MatchString(params[0]) || !hashPattern.MatchString(params[1]) || !hashPattern.MatchString(params[2]) {
		s.policy.ApplyMalformedPolicy(cs.ip)
		log.Printf("Malformed PoW result on %s from %s : %s %v", stratumConfig.Name, cs.ip, login, params)
		return false, &ErrorReply{Code: -1, Message: "Malformed PoW result"}
	}
	t := s.currentBlockTemplate()
	exist, valid, stale := s.processShare(login, id, cs.ip, t, params, cs.s_id)
	ok := s.policy.ApplySharePolicy(cs.ip, !exist && valid)
	
	if exist && valid {
		log.Printf("Duplicate share on %s from %s : %s %v", stratumConfig.Name, cs.ip, login, params)
		return false, &ErrorReply{Code: 22, Message: "Duplicate share"}
	}
	
	if stale {
		log.Printf("Stale share on %s from %s : %s %v", stratumConfig.Name, cs.ip, login, params)
		return false, nil
	}
	
	if !valid {
		log.Printf("Invalid share on %s from %s : %s %v", stratumConfig.Name, cs.ip, login, params)
		if !ok {
			return false, &ErrorReply{Code: 23, Message: "Invalid share"}
		}
		return false, nil
	}
	log.Printf("Valid share on %s from %s : %s %v", stratumConfig.Name, cs.ip, login, params)
	
	if !ok {
		return true, &ErrorReply{Code: -1, Message: "High rate of invalid or stale shares"}
	}
	return true, nil
}

func (s *ProxyServer) handleGetBlockByNumberRPC() *rpc.GetBlockReply {
	t := s.currentBlockTemplate()
	var reply *rpc.GetBlockReply
	if t != nil {
		reply = t.GetPendingBlockCache
	}
	return reply
}

func (s *ProxyServer) handleUnknownRPC(cs *Session, m string) *ErrorReply {
	stratumConfig := s.config.Proxy.Stratum[cs.s_id]
	log.Printf("Unknown request method on %s from %s : %s", stratumConfig.Name, cs.ip, m)
	s.policy.ApplyMalformedPolicy(cs.ip)
	return &ErrorReply{Code: -3, Message: "Method not found"}
}

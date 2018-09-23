package proxy

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/NotoriousPyro/open-metaverse-pool/util"
)

const (
	MaxReqSize = 1024
)

func (s *ProxyServer) ListenTCP(s_id int) {
	stratumConfig := s.config.Proxy.Stratum[s_id]
	timeout := util.MustParseDuration(stratumConfig.Timeout)
	s.stratum[s_id].timeout = timeout

	addr, err := net.ResolveTCPAddr("tcp", stratumConfig.Listen)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	server, err := net.ListenTCP("tcp", addr)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer server.Close()
	
	log.Printf("Stratum %s listening on %s (Difficulty: %d)", stratumConfig.Name, stratumConfig.Listen, stratumConfig.Difficulty)
	var accept = make(chan int, stratumConfig.MaxConn)
	n := 0

	for {
		conn, err := server.AcceptTCP()
		if err != nil {
			continue
		}
		conn.SetKeepAlive(true)

		ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())

		if s.policy.IsBanned(ip) || !s.policy.ApplyLimitPolicy(ip) {
			conn.Close()
			continue
		}
		n += 1
		cs := &Session{s_id: s_id, conn: conn, ip: ip}

		accept <- n
		go func(cs *Session) {
			err = s.handleTCPClient(cs)
			if err != nil {
				s.removeSession(cs)
				conn.Close()
			}
			<-accept
		}(cs)
	}
}

func (s *ProxyServer) handleTCPClient(cs *Session) error {
	cs.enc = json.NewEncoder(cs.conn)
	connbuff := bufio.NewReaderSize(cs.conn, MaxReqSize)
	s.setDeadline(cs.conn, cs.s_id)
	stratumConfig := s.config.Proxy.Stratum[cs.s_id]
	for {
		data, isPrefix, err := connbuff.ReadLine()
		if isPrefix {
			log.Printf("Socket flood detected on %s from %s", stratumConfig.Name, cs.ip)
			s.policy.BanClient(cs.ip)
			return err
		} else if err == io.EOF {
			log.Printf("Client on %s disconnected: %s ", stratumConfig.Name, cs.ip)
			s.removeSession(cs)
			break
		} else if err != nil {
			log.Printf("Error reading from socket on %s: %v", stratumConfig.Name, err)
			return err
		}

		if len(data) > 1 {
			var req StratumReq
			err = json.Unmarshal(data, &req)
			if err != nil {
				s.policy.ApplyMalformedPolicy(cs.ip)
				log.Printf("Malformed stratum request on %s from %s: %v", stratumConfig.Name, cs.ip, err)
				return err
			}
			s.setDeadline(cs.conn, cs.s_id)
			err = cs.handleTCPMessage(s, &req)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (cs *Session) handleTCPMessage(s *ProxyServer, req *StratumReq) error {
	stratumConfig := s.config.Proxy.Stratum[cs.s_id]
	// Handle RPC methods
	switch req.Method {
		case "eth_submitLogin", "eth_login":
			var params []string
			err := json.Unmarshal(req.Params, &params)
			if err != nil {
				log.Println("Malformed stratum request params on %s from %s", stratumConfig.Name, cs.ip)
				return err
			}
			reply, errReply := s.handleLoginRPC(cs, params, req.Worker)
			if errReply != nil {
				return cs.sendTCPError(req.Id, errReply)
			}
			return cs.sendTCPResult(req.Id, reply)
		case "eth_getWork":
			reply, errReply := s.handleGetWorkRPC(cs)
			if errReply != nil {
				return cs.sendTCPError(req.Id, errReply)
			}
			return cs.sendTCPResult(req.Id, &reply)
		case "eth_submitWork":
			var params []string
			err := json.Unmarshal(req.Params, &params)
			if err != nil {
				log.Println("Malformed stratum request params on %s from %s", stratumConfig.Name, cs.ip)
				return err
			}
			reply, errReply := s.handleTCPSubmitRPC(cs, req.Worker, params)
			if errReply != nil {
				return cs.sendTCPError(req.Id, errReply)
			}
			return cs.sendTCPResult(req.Id, &reply)
		case "eth_submitHashrate":
			return cs.sendTCPResult(req.Id, true)
		default:
			errReply := s.handleUnknownRPC(cs, req.Method)
			return cs.sendTCPError(req.Id, errReply)
	}
}

func (cs *Session) sendTCPResult(id json.RawMessage, result interface{}) error {
	cs.Lock()
	defer cs.Unlock()

	message := JSONRpcResp{Id: id, Version: "2.0", Error: nil, Result: result}
	return cs.enc.Encode(&message)
}

func (cs *Session) pushNewJob(result interface{}) error {
	cs.Lock()
	defer cs.Unlock()
	// FIXME: Temporarily add ID for Claymore compliance
	message := JSONPushMessage{Version: "2.0", Result: result, Id: 0}
	return cs.enc.Encode(&message)
}

func (cs *Session) sendTCPError(id json.RawMessage, reply *ErrorReply) error {
	cs.Lock()
	defer cs.Unlock()

	message := JSONRpcResp{Id: id, Version: "2.0", Error: reply}
	err := cs.enc.Encode(&message)
	if err != nil {
		return err
	}
	return errors.New(reply.Message)
}

func (self *ProxyServer) setDeadline(conn *net.TCPConn, s_id int) {
	conn.SetDeadline(time.Now().Add(self.stratum[s_id].timeout))
}

func (s *ProxyServer) registerSession(cs *Session) {
	stratum := s.stratum[cs.s_id]
	stratum.sessionsMu.Lock()
	defer stratum.sessionsMu.Unlock()
	stratum.sessions[cs] = struct{}{}
}

func (s *ProxyServer) removeSession(cs *Session) {
	stratum := s.stratum[cs.s_id]
	stratum.sessionsMu.Lock()
	defer stratum.sessionsMu.Unlock()
	delete(stratum.sessions, cs)
}

func (s *ProxyServer) broadcastNewJobs(s_id int) {
	proxyConfig := s.config.Proxy
	stratumConfig := proxyConfig.Stratum[s_id]
	t := s.currentBlockTemplate()
	if t == nil || len(t.Header) == 0 || s.isSick() {
		return
	}
	stratum := s.stratum[s_id]
	reply := []string{t.Header, t.Seed, stratum.diff}

	stratum.sessionsMu.RLock()
	defer stratum.sessionsMu.RUnlock()

	count := len(stratum.sessions)
	log.Printf("Broadcasting new job to %v miners on %s", count, stratumConfig.Name)
	s.backend.WriteStratumState(proxyConfig.Name, stratumConfig.Name, stratumConfig.Listen, count, stratumConfig.Difficulty)
	
	start := time.Now()
	bcast := make(chan int, 1024)
	n := 0

	for m, _ := range stratum.sessions {
		n++
		bcast <- n

		go func(cs *Session) {
			err := cs.pushNewJob(&reply)
			<-bcast
			if err != nil {
				log.Printf("Job transmit error from %s to %v@%v: %v", stratumConfig.Name, cs.login, cs.ip, err)
				s.removeSession(cs)
			} else {
				s.setDeadline(cs.conn, cs.s_id)
			}
		}(m)
	}
	log.Printf("Jobs broadcast on %s finished in %s", stratumConfig.Name, time.Since(start))
}

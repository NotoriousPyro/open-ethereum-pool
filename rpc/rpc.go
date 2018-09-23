package rpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/NotoriousPyro/open-metaverse-pool/util"
)

type RPCClient struct {
	sync.RWMutex
	Url				string
	Name			string
	Account			string
	Password		string
	sick			bool
	sickRate		int
	successRate		int
	client			*http.Client
}

type JSONRpcResp struct {
	Id				*json.RawMessage			`json:"id"`
	Result			*json.RawMessage			`json:"result"`
	Peers			*json.RawMessage			`json:"peers"`
	Error			map[string]interface{}		`json:"error"`
}

type GetBalanceReply struct {
	Unspent			int64	`json:"unspent"`
}

type ValidateAddress struct {
	IsValid			bool		`json:"is_valid"`
	TestNet			bool		`json:"testnet"`
}

type GetBlockReply struct {
	Difficulty			string		`json:"bits"`
	Hash				string		`json:"hash"`
	MerkleTreeHash		string		`json:"merkle_tree_hash"`
	Mixhash				string		`json:"mixhash"`
	Nonce				string		`json:"nonce"`
	Number				uint64		`json:"number"`
	PrevHash			string		`json:"previous_block_hash"`
	TimeStamp			uint64		`json:"timestamp"`
	TransactionCount	uint64		`json:"transaction_count"`
	Transactions		[]MVSTx		`json:"transactions"`
}

type MVSTx struct {
	Hash		string `json:"hash"`
	Locktime	string `json:"lock_time"`
	Outputs		[]MVSTxOutput `json:"outputs"`
}

type MVSTxOutput struct {
	Address		string `json:"address"`
	Value		int64 `json:"value"`
}

func (r *GetBlockReply) Confirmed() bool {
	return len(r.Hash) != 0
}

func (r *ValidateAddress) Valid() bool {
	return r.IsValid == true && r.TestNet == false
}

func NewRPCClient(name, url, account, password, timeout string) *RPCClient {
	rpcClient := &RPCClient{Name: name, Url: url, Account: account, Password: password}
	timeoutIntv := util.MustParseDuration(timeout)
	rpcClient.client = &http.Client{
		Timeout: timeoutIntv,
	}
	return rpcClient
}

func (r *RPCClient) GetWork() ([]string, error) {
	rpcResp, err := r.doPost(r.Url, "getwork", []string{})
	if err != nil {
		return nil, err
	}
	var reply []string
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return reply, err
}

func (r *RPCClient) SubmitWork(params []string) (bool, error) {
	rpcResp, err := r.doPost(r.Url, "submitwork", params)
	if err != nil {
		return false, err
	}
	var reply bool
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return reply, err
}

func (r *RPCClient) SetAddress(address string) ([]string, error) {
	rpcResp, err := r.doPost(r.Url, "setminingaccount", []string{r.Account, r.Password, address})
	if err != nil {
		return nil, err
	}
	var reply []string
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return reply, err
}

func (r *RPCClient) ValidateAddress(address string) (*ValidateAddress, error) {
	rpcResp, err := r.doPost(r.Url, "validateaddress", []string{address})
	if err != nil {
		return nil, err
	}
	var reply *ValidateAddress
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return reply, err
}

func (r *RPCClient) GetHeight() (uint64, error) {
	rpcResp, err := r.doPost(r.Url, "getheight", []string{})
	if err != nil {
		return 0, err
	}
	var reply uint64
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return reply, err
}

func (r *RPCClient) GetPendingBlock() (*GetBlockReply, error) {
	rpcResp, err := r.doPost(r.Url, "fetchheaderext", []string{r.Account, r.Password, "pending"})
	if err != nil {
		return nil, err
	}
	var reply *GetBlockReply
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return reply, err
}

func (r *RPCClient) GetBlockByHeight(height int64) (*GetBlockReply, error) {
	params := []interface{}{"-t", height}
	return r.GetBlockBy("getblockheader", params)
}

func (r *RPCClient) GetBlockByHash(hash string) (*GetBlockReply, error) {
	params := []interface{}{"-s", hash}
	return r.GetBlockBy("getblockheader", params)
}

func (r *RPCClient) GetBlockBy(method string, params []interface{}) (*GetBlockReply, error) {
	rpcResp, err := r.doPost(r.Url, method, params)
	if err != nil {
		return nil, err
	}
	var reply *GetBlockReply
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return reply, err
}

func (r *RPCClient) GetBlockTxs(height uint64) (*GetBlockReply, error) {
	rpcResp, err := r.doPost(r.Url, "getblock", []interface{}{height})
	if err != nil {
		return nil, err
	}
	var reply *GetBlockReply
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return reply, err
}

func (r *RPCClient) SendTransaction(from, to, value string) (string, error) {
	rpcResp, err := r.doPost(r.Url, "sendfrom", []string{r.Account, r.Password, from, to, value})
	if err != nil {
		return "", err
	}
	var reply MVSTx
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return reply.Hash, err
}

func (r *RPCClient) GetTransaction(hash string) (*GetBlockReply, error) {
	rpcResp, err := r.doPost(r.Url, "gettx", []string{hash})
	if err != nil {
		return nil, err
	}
	var reply *GetBlockReply
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return reply, err
}

func (r *RPCClient) GetBalance(address string) (*GetBalanceReply, error) {
	rpcResp, err := r.doPost(r.Url, "fetch-balance", []string{address})
	if err != nil {
		return nil, err
	}
	
	var reply *GetBalanceReply
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return reply, err
}

func (r *RPCClient) GetPeerCount() (int, error) {
	rpcResp, err := r.doPost(r.Url, "getpeerinfo", []string{})
	if err != nil {
		return 0, err
	}
	var reply []string
	err = json.Unmarshal(*rpcResp.Result, &reply)
	return len(reply), err
}

func (r *RPCClient) doPost(url string, method string, params interface{}) (*JSONRpcResp, error) {
	jsonReq := map[string]interface{}{"jsonrpc": "2.0", "method": method, "params": params, "id": 0}
	data, _ := json.Marshal(jsonReq)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	
	resp, err := r.client.Do(req)
	if err != nil {
		r.markSick()
		return nil, err
	}
	defer resp.Body.Close()

	var rpcResp *JSONRpcResp
	err = json.NewDecoder(resp.Body).Decode(&rpcResp)
	if err != nil {
		r.markSick()
		return nil, err
	}
	if rpcResp.Error != nil {
		r.markSick()
		return nil, errors.New(rpcResp.Error["message"].(string))
	}
	return rpcResp, err
}

func (r *RPCClient) Check() bool {
	_, err := r.GetWork()
	if err != nil {
		return false
	}
	r.markAlive()
	return !r.Sick()
}

func (r *RPCClient) Sick() bool {
	r.RLock()
	defer r.RUnlock()
	return r.sick
}

func (r *RPCClient) markSick() {
	r.Lock()
	r.sickRate++
	r.successRate = 0
	if r.sickRate >= 5 {
		r.sick = true
	}
	r.Unlock()
}

func (r *RPCClient) markAlive() {
	r.Lock()
	r.successRate++
	if r.successRate >= 5 {
		r.sick = false
		r.sickRate = 0
		r.successRate = 0
	}
	r.Unlock()
}
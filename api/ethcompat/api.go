// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package ethcompat implements an Ethereum JSON-RPC 2.0 compatible HTTP server for VeChain Thor.
// It exposes the subset of Ethereum methods required by hardhat, foundry, and cast.
// Only EIP-1559 (type=0x02) transactions are surfaced; VeChain-native transactions are hidden.
package ethcompat

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

// EthRPC is an Ethereum JSON-RPC 2.0 compatible handler for VeChain Thor.
type EthRPC struct {
	repo         *chain.Repository
	stater       *state.Stater
	pool         txpool.Pool
	logDB        *logdb.LogDB
	bft          bft.Committer
	forkConfig   *thor.ForkConfig
	chainID      uint64
	callGasLimit uint64
	version      string

	noncesMu sync.Mutex
	nonces   map[thor.Address]uint64
}

// New creates a new EthRPC handler.
func New(
	repo *chain.Repository,
	stater *state.Stater,
	pool txpool.Pool,
	logDB *logdb.LogDB,
	bft bft.Committer,
	forkConfig *thor.ForkConfig,
	callGasLimit uint64,
	version string,
) *EthRPC {
	chainID := thor.GetEthChainID(repo.GenesisBlock().Header().ID())
	return &EthRPC{
		repo:         repo,
		stater:       stater,
		pool:         pool,
		logDB:        logDB,
		bft:          bft,
		forkConfig:   forkConfig,
		chainID:      chainID,
		callGasLimit: callGasLimit,
		version:      version,
		nonces:       make(map[thor.Address]uint64),
	}
}

// ServeHTTP implements http.Handler — accepts JSON-RPC 2.0 POST requests.
func (e *EthRPC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body := json.NewDecoder(r.Body)

	// Peek at the first byte to determine single vs batch request.
	var raw json.RawMessage
	if err := body.Decode(&raw); err != nil {
		writeJSON(w, newErrResponse(nil, codeParseError, "parse error"))
		return
	}

	// Batch request: JSON array.
	if len(raw) > 0 && raw[0] == '[' {
		var reqs []rpcRequest
		if err := json.Unmarshal(raw, &reqs); err != nil {
			writeJSON(w, newErrResponse(nil, codeParseError, "parse error"))
			return
		}
		responses := make([]*rpcResponse, len(reqs))
		for i, req := range reqs {
			responses[i] = e.handle(req)
		}
		writeJSON(w, responses)
		return
	}

	// Single request.
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeJSON(w, newErrResponse(nil, codeParseError, "parse error"))
		return
	}
	writeJSON(w, e.handle(req))
}

// handle dispatches a single JSON-RPC request to the appropriate handler.
func (e *EthRPC) handle(req rpcRequest) *rpcResponse {
	result, err := e.dispatch(req.Method, req.Params)
	if err != nil {
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: err}
	}
	return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

// dispatch routes the method name to the correct handler function.
func (e *EthRPC) dispatch(method string, params json.RawMessage) (any, *rpcError) {
	p, err := parseParams(params)
	if err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: err.Error()}
	}

	switch method {
	// Network / meta
	case "web3_clientVersion":
		return e.clientVersion()
	case "net_version":
		return e.netVersion()
	case "net_listening":
		return true, nil
	case "eth_chainId":
		return e.ethChainID()

	// Block queries
	case "eth_blockNumber":
		return e.ethBlockNumber()
	case "eth_getBlockByHash":
		return e.ethGetBlockByHash(p)
	case "eth_getBlockByNumber":
		return e.ethGetBlockByNumber(p)

	// State queries
	case "eth_getBalance":
		return e.ethGetBalance(p)
	case "eth_getCode":
		return e.ethGetCode(p)
	case "eth_getStorageAt":
		return e.ethGetStorageAt(p)
	case "eth_getTransactionCount":
		return e.ethGetTransactionCount(p)

	// Call / estimate
	case "eth_call":
		return e.ethCall(p)
	case "eth_estimateGas":
		return e.ethEstimateGas(p)

	// Fee helpers
	case "eth_gasPrice":
		return e.ethGasPrice()
	case "eth_maxPriorityFeePerGas":
		return e.ethMaxPriorityFeePerGas()
	case "eth_feeHistory":
		return e.ethFeeHistory(p)

	// Transaction submission and query
	case "eth_sendRawTransaction":
		return e.ethSendRawTransaction(p)
	case "eth_getTransactionByHash":
		return e.ethGetTransactionByHash(p)
	case "eth_getTransactionReceipt":
		return e.ethGetTransactionReceipt(p)

	// Logs
	case "eth_getLogs":
		return e.ethGetLogs(p)

	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + method}
	}
}

// getNonce returns the current in-memory nonce for addr.
func (e *EthRPC) getNonce(addr thor.Address) uint64 {
	e.noncesMu.Lock()
	defer e.noncesMu.Unlock()
	return e.nonces[addr]
}

// incrementNonce increments and returns the new nonce for addr.
func (e *EthRPC) incrementNonce(addr thor.Address) uint64 {
	e.noncesMu.Lock()
	defer e.noncesMu.Unlock()
	n := e.nonces[addr] + 1
	e.nonces[addr] = n
	return n
}

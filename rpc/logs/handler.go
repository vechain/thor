// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/ethconvert"
	"github.com/vechain/thor/v2/rpc/jsonrpc"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Handler implements the eth_getLogs JSON-RPC method.
type Handler struct {
	repo      *chain.Repository
	logDB     *logdb.LogDB
	backtrace uint32
	logsLimit uint64
}

// New creates a logs Handler.
func New(repo *chain.Repository, logDB *logdb.LogDB, backtrace uint32, logsLimit uint64) *Handler {
	return &Handler{repo: repo, logDB: logDB, backtrace: backtrace, logsLimit: logsLimit}
}

// Mount registers all log methods on the dispatcher.
func (h *Handler) Mount(s *jsonrpc.Server) {
	s.Register("eth_getLogs", h.ethGetLogs)
}

func (h *Handler) ethGetLogs(req jsonrpc.Request) jsonrpc.Response {
	var params []rpc.EthLogFilter
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "expected [filterObject]")
	}
	f := params[0]

	// Single BestBlockSummary read so bestChain and bestNum are always consistent.
	bestSummary := h.repo.BestBlockSummary()
	bestChain := h.repo.NewChain(bestSummary.Header.ID())
	bestNum := bestSummary.Header.Number()

	var fromNum, toNum uint32

	if f.BlockHash != nil {
		// EIP-234: blockHash is mutually exclusive with fromBlock/toBlock.
		if (f.FromBlock != nil && *f.FromBlock != "") || (f.ToBlock != nil && *f.ToBlock != "") {
			return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "can't specify fromBlock/toBlock with blockHash")
		}
		summary, err := ethconvert.ResolveBlockTag(*f.BlockHash, h.repo)
		if err != nil {
			return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeServerError, "unknown block")
		}
		fromNum = summary.Header.Number()
		toNum = summary.Header.Number()
	} else {
		// Per Ethereum spec, absent fromBlock and toBlock both default to "latest".
		fromNum = bestNum
		toNum = bestNum

		if f.FromBlock != nil && *f.FromBlock != "" {
			summary, err := ethconvert.ResolveBlockTag(*f.FromBlock, h.repo)
			if err != nil {
				return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid fromBlock")
			}
			fromNum = summary.Header.Number()
		}
		if f.ToBlock != nil && *f.ToBlock != "" {
			summary, err := ethconvert.ResolveBlockTag(*f.ToBlock, h.repo)
			if err != nil {
				return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid toBlock")
			}
			toNum = summary.Header.Number()
		}

		if toNum > bestNum {
			toNum = bestNum
		}
		if fromNum > toNum {
			return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid block range params")
		}
		if toNum-fromNum > h.backtrace {
			return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeServerError, fmt.Sprintf("block range exceeds backtrace limit of %d", h.backtrace))
		}
	}

	// Parse address filter
	var addresses []*thor.Address
	if len(f.Address) > 0 {
		var single string
		var multi []string
		if err := json.Unmarshal(f.Address, &single); err == nil {
			addr, err := thor.ParseAddress(single)
			if err != nil {
				return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid address in filter")
			}
			a := addr
			addresses = append(addresses, &a)
		} else if err := json.Unmarshal(f.Address, &multi); err == nil {
			for _, s := range multi {
				addr, err := thor.ParseAddress(s)
				if err != nil {
					return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid address in filter")
				}
				a := addr
				addresses = append(addresses, &a)
			}
		}
	}

	// Parse topic filters — up to 5 positions (topic0…topic4), each null | hex | []hex.
	// Adjacent positions are ANDed; alternatives within one position are ORed.
	// topicAlts[i] holds all accepted values for position i; empty means wildcard.
	var topicAlts [5][]thor.Bytes32
	topics := f.Topics
	if len(topics) > len(topicAlts) {
		topics = topics[:len(topicAlts)]
	}
	for i, raw := range topics {
		if raw == nil || string(raw) == "null" {
			continue // nil = wildcard for this position
		}
		var single string
		var multi []string
		if err := json.Unmarshal(raw, &single); err == nil {
			h32, err := rpc.ParseBytes32Compact(single)
			if err != nil {
				return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid topic")
			}
			topicAlts[i] = []thor.Bytes32{h32}
		} else if err := json.Unmarshal(raw, &multi); err == nil && len(multi) > 0 {
			alts := make([]thor.Bytes32, 0, len(multi))
			for _, s := range multi {
				h32, err := rpc.ParseBytes32Compact(s)
				if err != nil {
					return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInvalidParams, "invalid topic")
				}
				alts = append(alts, h32)
			}
			topicAlts[i] = alts
		}
	}

	// Build criteria set via cross-product of addresses × topic alternatives.
	// Criteria count grows as the product of per-position alternative counts and
	// address count; typical usage is small and no hard cap is enforced.
	criteriaSet := buildCriteriaSet(addresses, topicAlts)

	// Fetch one extra result to detect truncation: if the logdb returns more than
	// logsLimit rows, return an error instead of a silently incomplete result.
	queryLimit := h.logsLimit
	if queryLimit < math.MaxUint64 {
		queryLimit++
	}
	filter := &logdb.EventFilter{
		CriteriaSet: criteriaSet,
		Range: &logdb.Range{
			From: fromNum,
			To:   toNum,
		},
		Options: &logdb.Options{Limit: queryLimit},
		Order:   logdb.ASC,
	}

	events, err := h.logDB.FilterEvents(context.Background(), filter)
	if err != nil {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
	}
	if uint64(len(events)) > h.logsLimit {
		return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeServerError,
			fmt.Sprintf("query returned more than %d results, use a smaller block range or a more specific filter", h.logsLimit))
	}

	// Post-filter: only return logs from TypeEthDynamicFee transactions.
	// Projected transactionIndex and logIndex are computed relative to ETH-typed txs only,
	// so that they remain consistent with eth_getTransactionByHash etc. in mixed blocks.
	//
	// blockTxsByNum caches the full tx list per block (one GetBlock call per unique block).
	// ethProjTxIdx caches canonical-position → projected-ETH-index per tx (avoids recount
	// when the same tx emits multiple events).
	// ethLogIdxByBlock counts ETH events seen so far per block (becomes the projected logIndex).
	blockTxsByNum := make(map[uint32][]*tx.Transaction)
	ethProjTxIdx := make(map[thor.Bytes32]uint32)
	ethLogIdxByBlock := make(map[thor.Bytes32]uint32)

	getBlockTxs := func(blockNum uint32) ([]*tx.Transaction, error) {
		if txs, ok := blockTxsByNum[blockNum]; ok {
			return txs, nil
		}
		blk, err := bestChain.GetBlock(blockNum)
		if err != nil {
			return nil, err
		}
		txs := blk.Transactions()
		blockTxsByNum[blockNum] = txs
		return txs, nil
	}

	var ethLogs []*rpc.EthLog
	// blockTxsByNum caches block transactions per block number so GetBlock is called
	// at most once per unique block in the result set, not once per event.
	for _, ev := range events {
		blockTxs, err := getBlockTxs(ev.BlockNumber)
		if err != nil {
			return jsonrpc.ErrResponse(req.ID, jsonrpc.CodeInternalError, err.Error())
		}

		// Bounds-check and ID-verify before using the canonical index.
		if int(ev.TxIndex) >= len(blockTxs) || blockTxs[ev.TxIndex].ID() != ev.TxID {
			continue
		}
		if blockTxs[ev.TxIndex].Type() != tx.TypeEthDynamicFee {
			continue
		}

		// Projected ETH tx index: number of TypeEthDynamicFee txs at canonical positions < ev.TxIndex.
		projTxIdx, ok := ethProjTxIdx[ev.TxID]
		if !ok {
			for i := uint32(0); i < ev.TxIndex; i++ {
				if blockTxs[i].Type() == tx.TypeEthDynamicFee {
					projTxIdx++
				}
			}
			ethProjTxIdx[ev.TxID] = projTxIdx
		}

		// Projected ETH log index: running count of ETH events in this block so far.
		logIdx := ethLogIdxByBlock[ev.BlockID]
		ethLogIdxByBlock[ev.BlockID]++

		evTopics := make([]common.Hash, 0, 5)
		for _, tp := range ev.Topics {
			if tp == nil {
				break
			}
			evTopics = append(evTopics, common.Hash(*tp))
		}

		ethLogs = append(ethLogs, &rpc.EthLog{
			Address:     common.Address(ev.Address),
			Topics:      evTopics,
			Data:        ev.Data,
			BlockNumber: hexutil.Uint64(ev.BlockNumber),
			TxHash:      common.Hash(ev.TxID),
			TxIndex:     hexutil.Uint64(projTxIdx),
			BlockHash:   common.Hash(ev.BlockID),
			LogIndex:    hexutil.Uint64(logIdx),
			Removed:     false,
		})
	}
	if ethLogs == nil {
		ethLogs = []*rpc.EthLog{}
	}
	return jsonrpc.OkResponse(req.ID, ethLogs)
}

// buildCriteriaSet returns the EventCriteria cross-product for the given addresses
// and per-slot topic alternatives. Positions with no alternatives are wildcards (Topics[i] == nil).
func buildCriteriaSet(addresses []*thor.Address, topicAlts [5][]thor.Bytes32) []*logdb.EventCriteria {
	type topicCombo [5]*thor.Bytes32
	combos := []topicCombo{{}}
	for i, alts := range topicAlts {
		if len(alts) == 0 {
			continue
		}
		expanded := make([]topicCombo, 0, len(combos)*len(alts))
		for _, c := range combos {
			for _, alt := range alts {
				newCombo := c
				altCopy := alt
				newCombo[i] = &altCopy
				expanded = append(expanded, newCombo)
			}
		}
		combos = expanded
	}
	var criteria []*logdb.EventCriteria
	if len(addresses) == 0 {
		for _, c := range combos {
			topics := c
			criteria = append(criteria, &logdb.EventCriteria{Topics: topics})
		}
	} else {
		for _, addr := range addresses {
			for _, c := range combos {
				addrCopy := *addr
				topics := c
				criteria = append(criteria, &logdb.EventCriteria{Address: &addrCopy, Topics: topics})
			}
		}
	}
	return criteria
}

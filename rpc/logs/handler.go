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
func (h *Handler) Mount(s *rpc.Server) {
	s.Register("eth_getLogs", h.ethGetLogs)
}

// LogFilter mirrors the Ethereum eth_getLogs filter parameter.
type LogFilter struct {
	FromBlock *string           `json:"fromBlock"`
	ToBlock   *string           `json:"toBlock"`
	Address   json.RawMessage   `json:"address"`   // string | []string | null
	Topics    []json.RawMessage `json:"topics"`    // each: null | string | []string
	BlockHash *string           `json:"blockHash"` // EIP-234: mutually exclusive with from/toBlock
}

func (h *Handler) ethGetLogs(req rpc.Request) rpc.Response {
	var params []LogFilter
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params) < 1 {
		return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "expected [filterObject]")
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
			return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "can't specify fromBlock/toBlock with blockHash")
		}
		summary, err := rpc.ResolveBlockTag(*f.BlockHash, h.repo)
		if err != nil {
			return rpc.ErrResponse(req.ID, rpc.CodeServerError, "unknown block")
		}
		fromNum = summary.Header.Number()
		toNum = summary.Header.Number()
	} else {
		// Per Ethereum spec, absent fromBlock and toBlock both default to "latest".
		fromNum = bestNum
		toNum = bestNum

		if f.FromBlock != nil && *f.FromBlock != "" {
			summary, err := rpc.ResolveBlockTag(*f.FromBlock, h.repo)
			if err != nil {
				return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid fromBlock")
			}
			fromNum = summary.Header.Number()
		}
		if f.ToBlock != nil && *f.ToBlock != "" {
			summary, err := rpc.ResolveBlockTag(*f.ToBlock, h.repo)
			if err != nil {
				return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid toBlock")
			}
			toNum = summary.Header.Number()
		}

		if toNum > bestNum {
			toNum = bestNum
		}
		if fromNum > toNum {
			return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid block range params")
		}
		if toNum-fromNum > h.backtrace {
			return rpc.ErrResponse(req.ID, rpc.CodeServerError, fmt.Sprintf("block range exceeds backtrace limit of %d", h.backtrace))
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
				return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid address in filter")
			}
			a := addr
			addresses = append(addresses, &a)
		} else if err := json.Unmarshal(f.Address, &multi); err == nil {
			for _, s := range multi {
				addr, err := thor.ParseAddress(s)
				if err != nil {
					return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid address in filter")
				}
				a := addr
				addresses = append(addresses, &a)
			}
		}
	}

	// Parse topic filters — up to 5 positions (topic0…topic4), each null | hex | []hex.
	// Adjacent positions are ANDed: topics: ["A", "B"] means topic0==A AND topic1==B.
	// OR semantics within one position (topics: [["A","C"], "B"]) are not yet fully
	// supported — only the first alternative is used.
	// TODO: full OR-within-position support requires expanding into a cross-product of
	// EventCriteria (one per combination of per-position alternatives).
	var topicSlot [5]*thor.Bytes32
	topics := f.Topics
	if len(topics) > len(topicSlot) {
		topics = topics[:len(topicSlot)]
	}
	for i, raw := range topics {
		if raw == nil || string(raw) == "null" {
			continue // nil = wildcard for this position
		}
		var single string
		var multi []string
		if err := json.Unmarshal(raw, &single); err == nil {
			h32, err := thor.ParseBytes32(single)
			if err != nil {
				return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid topic")
			}
			h32Copy := h32
			topicSlot[i] = &h32Copy
		} else if err := json.Unmarshal(raw, &multi); err == nil && len(multi) > 0 {
			h32, err := thor.ParseBytes32(multi[0])
			if err != nil {
				return rpc.ErrResponse(req.ID, rpc.CodeInvalidParams, "invalid topic")
			}
			h32Copy := h32
			topicSlot[i] = &h32Copy
		}
	}

	// Build criteria set: one EventCriteria per address with all topic positions ANDed.
	var criteriaSet []*logdb.EventCriteria
	buildCriteria := func(addr *thor.Address) {
		criteriaSet = append(criteriaSet, &logdb.EventCriteria{
			Address: addr,
			Topics:  topicSlot,
		})
	}

	if len(addresses) == 0 {
		buildCriteria(nil)
	} else {
		for _, addr := range addresses {
			a := addr
			buildCriteria(a)
		}
	}

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
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}
	if uint64(len(events)) > h.logsLimit {
		return rpc.ErrResponse(req.ID, rpc.CodeServerError,
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
	for _, ev := range events {
		blockTxs, err := getBlockTxs(ev.BlockNumber)
		if err != nil {
			return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
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
	return rpc.OkResponse(req.ID, ethLogs)
}

// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logs

import (
	"context"
	"encoding/json"
	"fmt"

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
}

// New creates a logs Handler.
func New(repo *chain.Repository, logDB *logdb.LogDB, backtrace uint32) *Handler {
	return &Handler{repo: repo, logDB: logDB, backtrace: backtrace}
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

	bestChain := h.repo.NewBestChain()
	bestNum := h.repo.BestBlockSummary().Header.Number()

	var fromNum, toNum uint32

	if f.BlockHash != nil {
		// EIP-234: single block identified by hash
		summary, err := rpc.ResolveBlockTag(*f.BlockHash, h.repo)
		if err != nil {
			return rpc.OkResponse(req.ID, []*rpc.EthLog{})
		}
		fromNum = summary.Header.Number()
		toNum = summary.Header.Number()
	} else {
		// Default range
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
		if toNum > fromNum && toNum-fromNum > h.backtrace {
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

	filter := &logdb.EventFilter{
		CriteriaSet: criteriaSet,
		Range: &logdb.Range{
			From: fromNum,
			To:   toNum,
		},
		Options: &logdb.Options{Limit: 10000},
		Order:   logdb.ASC,
	}

	events, err := h.logDB.FilterEvents(context.Background(), filter)
	if err != nil {
		return rpc.ErrResponse(req.ID, rpc.CodeInternalError, err.Error())
	}

	// Post-filter: only return logs from TypeEthTyped1559 transactions.
	// Cache tx type lookups per unique TxID to avoid redundant chain reads.
	typeCache := make(map[thor.Bytes32]bool)
	isEthTx := func(txID thor.Bytes32) bool {
		if v, ok := typeCache[txID]; ok {
			return v
		}
		t, _, err := bestChain.GetTransaction(txID)
		ok2 := err == nil && t.Type() == tx.TypeEthDynamicFee
		typeCache[txID] = ok2
		return ok2
	}

	var ethLogs []*rpc.EthLog
	for _, ev := range events {
		if !isEthTx(ev.TxID) {
			continue
		}
		topics := make([]common.Hash, 0, 5)
		for _, tp := range ev.Topics {
			if tp == nil {
				break
			}
			topics = append(topics, common.Hash(*tp))
		}
		ethLogs = append(ethLogs, &rpc.EthLog{
			Address:     common.Address(ev.Address),
			Topics:      topics,
			Data:        ev.Data,
			BlockNumber: hexutil.Uint64(ev.BlockNumber),
			TxHash:      common.Hash(ev.TxID),
			TxIndex:     hexutil.Uint64(ev.TxIndex),
			BlockHash:   common.Hash(ev.BlockID),
			LogIndex:    hexutil.Uint64(ev.LogIndex),
			Removed:     false,
		})
	}
	if ethLogs == nil {
		ethLogs = []*rpc.EthLog{}
	}
	return rpc.OkResponse(req.ID, ethLogs)
}

// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
)

func init() {
	register("eth_getLogs", handleGetLogs)
}

// ethLogFilter is the eth_getLogs request shape. fromBlock/toBlock and
// blockHash are mutually exclusive per EIP-234.
type ethLogFilter struct {
	FromBlock *json.RawMessage `json:"fromBlock"`
	ToBlock   *json.RawMessage `json:"toBlock"`
	Address   json.RawMessage  `json:"address"`
	Topics    []json.RawMessage `json:"topics"`
	BlockHash *thor.Bytes32    `json:"blockHash"`
}

// ethLog is the eth-shape log entry. `transactionHash` is the canonical tx
// id (Spec 1 invariant).
type ethLog struct {
	Address          thor.Address     `json:"address"`
	Topics           []thor.Bytes32   `json:"topics"`
	Data             hexutil.Bytes    `json:"data"`
	BlockNumber      hexutil.Uint64   `json:"blockNumber"`
	BlockHash        thor.Bytes32     `json:"blockHash"`
	TransactionHash  thor.Bytes32     `json:"transactionHash"`
	TransactionIndex hexutil.Uint64   `json:"transactionIndex"`
	LogIndex         hexutil.Uint64   `json:"logIndex"`
	Removed          bool             `json:"removed"`
}

// handleGetLogs filters events from logdb and projects them into eth-shape
// log entries. Enforces the range cap (APIBacktraceLimit) and result cap
// (LogsLimit); overrun surfaces as log_range_too_large per spec §7.2.
func handleGetLogs(ctx context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var raw []json.RawMessage
	if err := json.Unmarshal(params, &raw); err != nil {
		return nil, InvalidParams("params must be [filter]")
	}
	if len(raw) != 1 {
		return nil, InvalidParams("expected [filter]")
	}

	var f ethLogFilter
	if err := json.Unmarshal(raw[0], &f); err != nil {
		return nil, InvalidParams("filter: " + err.Error())
	}

	// blockHash and fromBlock/toBlock are mutually exclusive.
	if f.BlockHash != nil && (f.FromBlock != nil || f.ToBlock != nil) {
		return nil, InvalidParams("filter: blockHash and fromBlock/toBlock are mutually exclusive")
	}

	// Resolve block range.
	var fromNum, toNum uint32
	if f.BlockHash != nil {
		// Pin to a single block; also honor canonical check.
		bestChain := s.repo.NewBestChain()
		sum, err := s.repo.GetBlockSummary(*f.BlockHash)
		if err != nil {
			return nil, InvalidParams("filter.blockHash not found: " + f.BlockHash.String())
		}
		canonicalID, cerr := bestChain.GetBlockID(sum.Header.Number())
		if cerr != nil || canonicalID != *f.BlockHash {
			return nil, ReasonError(ReasonBlockNotCanonical, "blockHash is not on the canonical chain: "+f.BlockHash.String())
		}
		fromNum = sum.Header.Number()
		toNum = fromNum
	} else {
		best := s.repo.BestBlockSummary().Header.Number()
		from, rerr := resolveLogBlockBound(s, f.FromBlock, 0)
		if rerr != nil {
			return nil, rerr
		}
		to, rerr := resolveLogBlockBound(s, f.ToBlock, best)
		if rerr != nil {
			return nil, rerr
		}
		if from > to {
			return nil, InvalidParams("filter: fromBlock > toBlock")
		}
		fromNum, toNum = from, to
	}

	// Range cap.
	if cap := s.cfg.APIBacktraceLimit; cap > 0 {
		span := int(toNum) - int(fromNum) + 1
		if span > cap {
			return nil, ReasonError(ReasonLogRangeTooLarge, fmt.Sprintf("requested range %d blocks exceeds cap %d", span, cap))
		}
	}

	// Topics validation: eth-spec caps at 4 (topic0..topic3); topic4 is Thor-
	// only and unreachable via eth_getLogs so we hard-cap at 4.
	if len(f.Topics) > 4 {
		return nil, InvalidParams("filter.topics: at most 4 topic slots allowed")
	}

	// Translate to logdb filter.
	criteria, rerr := buildEventCriteriaSet(f.Address, f.Topics)
	if rerr != nil {
		return nil, rerr
	}
	filter := &logdb.EventFilter{
		CriteriaSet: criteria,
		Range:       &logdb.Range{From: fromNum, To: toNum},
		Order:       logdb.ASC,
	}
	if s.cfg.LogsLimit > 0 {
		// Ask for one over the limit so we can detect overrun precisely.
		filter.Options = &logdb.Options{Limit: s.cfg.LogsLimit + 1}
	}

	events, err := s.logDB.FilterEvents(ctx, filter)
	if err != nil {
		return nil, InternalError(err)
	}
	if s.cfg.LogsLimit > 0 && uint64(len(events)) > s.cfg.LogsLimit {
		return nil, ReasonError(ReasonLogRangeTooLarge, fmt.Sprintf("results exceed limit %d", s.cfg.LogsLimit))
	}

	out := make([]*ethLog, 0, len(events))
	for _, e := range events {
		topics := make([]thor.Bytes32, 0, 4)
		for _, t := range e.Topics {
			if t == nil {
				break
			}
			topics = append(topics, *t)
		}
		// Per Spec 1 invariant the on-wire transactionHash is the canonical
		// txid. logdb stores the native tx.ID(); translate via the chain
		// repository to the canonical form so wallets can cross-reference
		// what eth_sendRawTransaction returned.
		canonical := e.TxID
		if trx, _, gerr := s.repo.NewBestChain().GetTransaction(e.TxID); gerr == nil {
			canonical = trx.CanonicalTxID()
		}

		out = append(out, &ethLog{
			Address:          e.Address,
			Topics:           topics,
			Data:             append(hexutil.Bytes(nil), e.Data...),
			BlockNumber:      hexutil.Uint64(e.BlockNumber),
			BlockHash:        e.BlockID,
			TransactionHash:  canonical,
			TransactionIndex: hexutil.Uint64(e.TxIndex),
			LogIndex:         hexutil.Uint64(e.LogIndex),
			Removed:          false,
		})
	}
	return out, nil
}

// resolveLogBlockBound parses a fromBlock/toBlock tag. Nil rawtag means "use
// the default" (caller-supplied, usually best for toBlock and 0 for
// fromBlock). Supports named tags and hex quantities; bare hashes are not
// accepted on this axis (EIP-234 only permits blockHash at the filter level).
func resolveLogBlockBound(s *Server, rawtag *json.RawMessage, defaultVal uint32) (uint32, *RPCError) {
	if rawtag == nil || string(*rawtag) == "null" {
		return defaultVal, nil
	}
	var tag BlockTag
	if err := json.Unmarshal(*rawtag, &tag); err != nil {
		return 0, InvalidParams("block bound: " + err.Error())
	}
	// Disallow hash shape here.
	if tag.blockHash != nil {
		return 0, InvalidParams("fromBlock/toBlock must be a tag or number, not a hash; use filter.blockHash instead")
	}
	_, summary, err := tag.Resolve(s.repo, s.bft)
	if err != nil {
		return 0, ToRPCError(err)
	}
	return summary.Header.Number(), nil
}

// buildEventCriteriaSet translates the eth address + topics pair into the
// cross-product of EventCriteria that logdb expects. An `address` can be a
// single address or an array; each topic slot can be null, a single topic,
// or an array-of-alternatives.
//
// Eth's topic semantics: each slot i matches any of the listed topics at
// position i; slots combine as AND across positions. The cross-product of
// (addresses) × (topic0 alts) × (topic1 alts) × … expands into one
// EventCriteria per combination; logdb OR's them at the SQL level.
func buildEventCriteriaSet(addrRaw json.RawMessage, topicRaws []json.RawMessage) ([]*logdb.EventCriteria, *RPCError) {
	addrs, rerr := parseAddressField(addrRaw)
	if rerr != nil {
		return nil, rerr
	}

	// Parse each topic slot into a slice of alternatives (nil entry in the
	// outer slice means "any" for that position).
	topicSlots := make([][]*thor.Bytes32, len(topicRaws))
	for i, raw := range topicRaws {
		alts, err := parseTopicSlot(raw)
		if err != nil {
			return nil, InvalidParams(fmt.Sprintf("topics[%d]: %s", i, err.Error()))
		}
		topicSlots[i] = alts
	}

	// Address-free / topic-free fast path.
	if len(addrs) == 0 && len(topicSlots) == 0 {
		return nil, nil
	}

	// addressDims / topicDims — empty slots get a single "any" placeholder so
	// the cross-product includes them.
	addrDim := addrs
	if len(addrDim) == 0 {
		addrDim = []*thor.Address{nil}
	}
	// For each topic position, if the slot is empty ([]), treat as "any".
	normTopics := make([][]*thor.Bytes32, len(topicSlots))
	for i, s := range topicSlots {
		if len(s) == 0 {
			normTopics[i] = []*thor.Bytes32{nil}
		} else {
			normTopics[i] = s
		}
	}

	// Cross-product expansion.
	var out []*logdb.EventCriteria
	combos := combineTopics(normTopics)
	for _, a := range addrDim {
		for _, c := range combos {
			ec := &logdb.EventCriteria{Address: a}
			for i, t := range c {
				if i >= 5 {
					break
				}
				ec.Topics[i] = t
			}
			out = append(out, ec)
		}
	}
	return out, nil
}

// parseAddressField accepts null, a single 0x-address, or an array of
// addresses. Returns a slice of pointers suitable for the EventCriteria
// cross-product; a nil pointer means "any address".
func parseAddressField(raw json.RawMessage) ([]*thor.Address, *RPCError) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	// Try single address first.
	var single thor.Address
	if err := json.Unmarshal(raw, &single); err == nil {
		return []*thor.Address{&single}, nil
	}
	// Array form.
	var arr []thor.Address
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, InvalidParams("filter.address: " + err.Error())
	}
	out := make([]*thor.Address, len(arr))
	for i := range arr {
		out[i] = &arr[i]
	}
	return out, nil
}

// parseTopicSlot accepts null, a single topic, or an array of topics at a
// single position.
func parseTopicSlot(raw json.RawMessage) ([]*thor.Bytes32, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	// Single.
	var single thor.Bytes32
	if err := json.Unmarshal(raw, &single); err == nil {
		return []*thor.Bytes32{&single}, nil
	}
	// Array.
	var arr []thor.Bytes32
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}
	out := make([]*thor.Bytes32, len(arr))
	for i := range arr {
		out[i] = &arr[i]
	}
	return out, nil
}

// combineTopics returns every tuple picking exactly one element from each
// input slot. An empty input returns a single empty tuple.
func combineTopics(slots [][]*thor.Bytes32) [][]*thor.Bytes32 {
	if len(slots) == 0 {
		return [][]*thor.Bytes32{nil}
	}
	rest := combineTopics(slots[1:])
	out := make([][]*thor.Bytes32, 0, len(slots[0])*len(rest))
	for _, a := range slots[0] {
		for _, b := range rest {
			row := make([]*thor.Bytes32, 0, 1+len(b))
			row = append(row, a)
			row = append(row, b...)
			out = append(out, row)
		}
	}
	return out
}


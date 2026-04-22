// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethcompat

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
	"github.com/vechain/thor/v2/xenv"
)

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// resolveBlock resolves an Ethereum block parameter to a VeChain block summary and state.
func (e *EthRPC) resolveBlock(param string) (*block.Header, error) {
	rev, err := restutil.ParseRevision(ethBlockParamToRevision(param), false)
	if err != nil {
		return nil, err
	}
	sum, err := restutil.GetSummary(rev, e.repo, e.bft)
	if err != nil {
		return nil, err
	}
	return sum.Header, nil
}

// resolveBlockAndState resolves a block param to a header + state.
func (e *EthRPC) resolveBlockAndState(param string) (*block.Header, interface{ GetCode(thor.Address) ([]byte, error); GetStorage(thor.Address, thor.Bytes32) (thor.Bytes32, error) }, error) {
	rev, err := restutil.ParseRevision(ethBlockParamToRevision(param), false)
	if err != nil {
		return nil, nil, err
	}
	sum, st, err := restutil.GetSummaryAndState(rev, e.repo, e.bft, e.stater, e.forkConfig)
	if err != nil {
		return nil, nil, err
	}
	return sum.Header, st, nil
}

func rpcErr(code int, msg string) *rpcError {
	return &rpcError{Code: code, Message: msg}
}

func internalErr(err error) *rpcError {
	return rpcErr(codeInternalError, err.Error())
}

func invalidParams(msg string) *rpcError {
	return rpcErr(codeInvalidParams, msg)
}

// --------------------------------------------------------------------------
// Network / meta
// --------------------------------------------------------------------------

func (e *EthRPC) clientVersion() (any, *rpcError) {
	return "VeChain-Thor/" + e.version, nil
}

func (e *EthRPC) netVersion() (any, *rpcError) {
	return fmt.Sprintf("%d", e.chainID), nil
}

func (e *EthRPC) ethChainID() (any, *rpcError) {
	return hexutil.EncodeUint64(e.chainID), nil
}

// --------------------------------------------------------------------------
// Block queries
// --------------------------------------------------------------------------

func (e *EthRPC) ethBlockNumber() (any, *rpcError) {
	n := e.repo.BestBlockSummary().Header.Number()
	return hexutil.EncodeUint64(uint64(n)), nil
}

func (e *EthRPC) ethGetBlockByHash(params []json.RawMessage) (any, *rpcError) {
	hashStr, err := paramString(params, 0)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	fullTxs, _ := paramBool(params, 1)

	id, err := thor.ParseBytes32(hashStr)
	if err != nil {
		return nil, invalidParams("invalid block hash")
	}
	blk, err := e.repo.GetBlock(id)
	if err != nil {
		return nil, nil // not found → null
	}
	sum, err := e.repo.GetBlockSummary(id)
	if err != nil {
		return nil, internalErr(err)
	}
	return convertBlock(blk, sum, e.chainID, fullTxs), nil
}

func (e *EthRPC) ethGetBlockByNumber(params []json.RawMessage) (any, *rpcError) {
	numStr, err := paramString(params, 0)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	fullTxs, _ := paramBool(params, 1)

	header, err := e.resolveBlock(numStr)
	if err != nil {
		return nil, nil // not found → null
	}
	blk, err := e.repo.GetBlock(header.ID())
	if err != nil {
		return nil, nil
	}
	sum, err := e.repo.GetBlockSummary(header.ID())
	if err != nil {
		return nil, internalErr(err)
	}
	return convertBlock(blk, sum, e.chainID, fullTxs), nil
}

// --------------------------------------------------------------------------
// State queries
// --------------------------------------------------------------------------

func (e *EthRPC) ethGetBalance(params []json.RawMessage) (any, *rpcError) {
	addrStr, err := paramString(params, 0)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	blockParam := "latest"
	if len(params) > 1 {
		blockParam, _ = paramString(params, 1)
	}

	addr := common.HexToAddress(addrStr)
	thorAddr := ethAddrToThor(addr)

	rev, err := restutil.ParseRevision(ethBlockParamToRevision(blockParam), false)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	sum, st, err := restutil.GetSummaryAndState(rev, e.repo, e.bft, e.stater, e.forkConfig)
	if err != nil {
		return nil, internalErr(err)
	}

	energy, err := builtin.Energy.Native(st, sum.Header.Timestamp()).Get(thorAddr)
	if err != nil {
		return nil, internalErr(err)
	}
	return (*hexutil.Big)(energy), nil
}

func (e *EthRPC) ethGetCode(params []json.RawMessage) (any, *rpcError) {
	addrStr, err := paramString(params, 0)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	blockParam := "latest"
	if len(params) > 1 {
		blockParam, _ = paramString(params, 1)
	}

	addr := ethAddrToThor(common.HexToAddress(addrStr))
	rev, err := restutil.ParseRevision(ethBlockParamToRevision(blockParam), false)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	_, st, err := restutil.GetSummaryAndState(rev, e.repo, e.bft, e.stater, e.forkConfig)
	if err != nil {
		return nil, internalErr(err)
	}
	code, err := st.GetCode(addr)
	if err != nil {
		return nil, internalErr(err)
	}
	return hexutil.Bytes(code), nil
}

func (e *EthRPC) ethGetStorageAt(params []json.RawMessage) (any, *rpcError) {
	addrStr, err := paramString(params, 0)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	slotStr, err := paramString(params, 1)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	blockParam := "latest"
	if len(params) > 2 {
		blockParam, _ = paramString(params, 2)
	}

	addr := ethAddrToThor(common.HexToAddress(addrStr))
	slot, err := thor.ParseBytes32(slotStr)
	if err != nil {
		// Try padding short hex values.
		padded := strings.TrimPrefix(slotStr, "0x")
		for len(padded) < 64 {
			padded = "0" + padded
		}
		slot, err = thor.ParseBytes32("0x" + padded)
		if err != nil {
			return nil, invalidParams("invalid storage slot")
		}
	}

	rev, err := restutil.ParseRevision(ethBlockParamToRevision(blockParam), false)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	_, st, err := restutil.GetSummaryAndState(rev, e.repo, e.bft, e.stater, e.forkConfig)
	if err != nil {
		return nil, internalErr(err)
	}
	val, err := st.GetStorage(addr, slot)
	if err != nil {
		return nil, internalErr(err)
	}
	return hexutil.Bytes(val[:]), nil
}

func (e *EthRPC) ethGetTransactionCount(params []json.RawMessage) (any, *rpcError) {
	addrStr, err := paramString(params, 0)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	addr := ethAddrToThor(common.HexToAddress(addrStr))
	return hexutil.EncodeUint64(e.getNonce(addr)), nil
}

// --------------------------------------------------------------------------
// Call / estimate gas
// --------------------------------------------------------------------------

// executeCall runs a simulated transaction against the given block state.
func (e *EthRPC) executeCall(args callArgs, blockParam string) (*runtime.Output, error) {
	rev, err := restutil.ParseRevision(ethBlockParamToRevision(blockParam), false)
	if err != nil {
		return nil, err
	}
	sum, st, err := restutil.GetSummaryAndState(rev, e.repo, e.bft, e.stater, e.forkConfig)
	if err != nil {
		return nil, err
	}
	header := sum.Header

	// Build the clause.
	var toAddr *thor.Address
	if args.To != nil {
		a := ethAddrToThor(*args.To)
		toAddr = &a
	}
	var value *big.Int
	if args.Value != nil {
		value = args.Value.ToInt()
	} else {
		value = new(big.Int)
	}
	var data []byte
	if args.Input != nil {
		data = *args.Input
	} else if args.Data != nil {
		data = *args.Data
	}
	clause := tx.NewClause(toAddr).WithValue(value).WithData(data)

	// Caller.
	var origin thor.Address
	if args.From != nil {
		origin = ethAddrToThor(*args.From)
	}

	// Gas.
	gas := e.callGasLimit
	if args.Gas != nil && uint64(*args.Gas) > 0 {
		gas = uint64(*args.Gas)
		if gas > e.callGasLimit {
			gas = e.callGasLimit
		}
	}

	// Gas price.
	gasPrice := new(big.Int)
	if args.MaxFeePerGas != nil {
		gasPrice = args.MaxFeePerGas.ToInt()
	} else if args.GasPrice != nil {
		gasPrice = args.GasPrice.ToInt()
	} else if bf := header.BaseFee(); bf != nil {
		gasPrice = bf
	}

	signer, _ := header.Signer()
	rt := runtime.New(
		e.repo.NewChain(header.ParentID()),
		st,
		&xenv.BlockContext{
			Beneficiary: header.Beneficiary(),
			Signer:      signer,
			Number:      header.Number(),
			Time:        header.Timestamp(),
			GasLimit:    header.GasLimit(),
			TotalScore:  header.TotalScore(),
			BaseFee:     header.BaseFee(),
		},
		e.forkConfig,
	)

	txCtx := &xenv.TransactionContext{
		Origin:      origin,
		GasPayer:    origin,
		GasPrice:    gasPrice,
		ProvedWork:  new(big.Int),
		ClauseCount: 1,
	}

	exec, interrupt := rt.PrepareClause(clause, 0, gas, txCtx)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
			interrupt()
		case <-done:
		}
	}()
	out, _, err := exec()
	close(done)
	return out, err
}

func (e *EthRPC) ethCall(params []json.RawMessage) (any, *rpcError) {
	if len(params) == 0 {
		return nil, invalidParams("missing call object")
	}
	var args callArgs
	if err := json.Unmarshal(params[0], &args); err != nil {
		return nil, invalidParams(err.Error())
	}
	blockParam := "latest"
	if len(params) > 1 {
		blockParam, _ = paramString(params, 1)
	}

	out, err := e.executeCall(args, blockParam)
	if err != nil {
		return nil, internalErr(err)
	}
	if out.VMErr != nil {
		re := &rpcError{Code: codeExecutionError, Message: "execution reverted"}
		if len(out.Data) > 0 {
			re.Data = hexutil.Encode(out.Data)
		}
		return nil, re
	}
	return hexutil.Bytes(out.Data), nil
}

func (e *EthRPC) ethEstimateGas(params []json.RawMessage) (any, *rpcError) {
	if len(params) == 0 {
		return nil, invalidParams("missing call object")
	}
	var args callArgs
	if err := json.Unmarshal(params[0], &args); err != nil {
		return nil, invalidParams(err.Error())
	}
	blockParam := "latest"
	if len(params) > 1 {
		blockParam, _ = paramString(params, 1)
	}

	out, err := e.executeCall(args, blockParam)
	if err != nil {
		return nil, internalErr(err)
	}
	if out.VMErr != nil {
		re := &rpcError{Code: codeExecutionError, Message: "execution reverted"}
		if len(out.Data) > 0 {
			re.Data = hexutil.Encode(out.Data)
		}
		return nil, re
	}

	// PrepareClause runs the EVM directly and does NOT charge the VeChain
	// transaction-level intrinsic gas (TxGas + ClauseGas/ClauseGasContractCreation
	// + dataGas).  That deduction happens in ResolveTransaction before the EVM is
	// invoked.  We must add it back so the returned estimate covers the full gas
	// the txpool and packer will require.
	evmGasUsed := e.callGasLimit - out.LeftOverGas
	if args.Gas != nil && uint64(*args.Gas) < e.callGasLimit {
		evmGasUsed = uint64(*args.Gas) - out.LeftOverGas
	}

	// Reconstruct the clause to compute intrinsic gas without re-running the EVM.
	var intrinsicTo *thor.Address
	if args.To != nil {
		a := ethAddrToThor(*args.To)
		intrinsicTo = &a
	}
	var intrinsicData []byte
	if args.Input != nil {
		intrinsicData = *args.Input
	} else if args.Data != nil {
		intrinsicData = *args.Data
	}
	intrinsicClause := tx.NewClause(intrinsicTo).WithData(intrinsicData)
	intrinsic, _ := tx.IntrinsicGas(intrinsicClause)
	total := evmGasUsed + intrinsic

	estimated := total * 6 / 5 // 20% buffer
	if estimated < 21000 {
		estimated = 21000
	}
	return hexutil.EncodeUint64(estimated), nil
}

// --------------------------------------------------------------------------
// Fee helpers
// --------------------------------------------------------------------------

func (e *EthRPC) ethGasPrice() (any, *rpcError) {
	header := e.repo.BestBlockSummary().Header
	if bf := header.BaseFee(); bf != nil {
		return (*hexutil.Big)(bf), nil
	}
	return hexutil.EncodeUint64(0), nil
}

func (e *EthRPC) ethMaxPriorityFeePerGas() (any, *rpcError) {
	return hexutil.EncodeUint64(0), nil
}

func (e *EthRPC) ethFeeHistory(params []json.RawMessage) (any, *rpcError) {
	// Minimal implementation: return empty fee history.
	// Tools use this to determine gas settings; returning zeros causes them to fall back to defaults.
	header := e.repo.BestBlockSummary().Header
	var baseFee *hexutil.Big
	if bf := header.BaseFee(); bf != nil {
		baseFee = (*hexutil.Big)(bf)
	} else {
		baseFee = (*hexutil.Big)(new(big.Int))
	}
	return map[string]any{
		"oldestBlock":   hexutil.EncodeUint64(uint64(header.Number())),
		"baseFeePerGas": []any{baseFee},
		"gasUsedRatio":  []float64{},
		"reward":        []any{},
	}, nil
}

// --------------------------------------------------------------------------
// Transaction submission and query
// --------------------------------------------------------------------------

func (e *EthRPC) ethSendRawTransaction(params []json.RawMessage) (any, *rpcError) {
	rawHex, err := paramString(params, 0)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	rawBytes, err := hexutil.Decode(rawHex)
	if err != nil {
		return nil, invalidParams("invalid hex: " + err.Error())
	}

	// Use NormalizeEthereumTx — the proper entry point that validates chain ID,
	// signature, and field ranges before pool ingestion.
	norm, normErr := tx.NormalizeEthereumTx(rawBytes, e.chainID)
	if normErr != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "invalid transaction: " + normErr.Error()}
	}
	transaction := tx.NewEthereumTransaction(norm)

	if err := e.pool.AddLocal(transaction); err != nil {
		if txpool.IsBadTx(err) {
			return nil, &rpcError{Code: codeInvalidParams, Message: err.Error()}
		}
		return nil, &rpcError{Code: codeInternalError, Message: err.Error()}
	}

	e.incrementNonce(norm.Sender)
	return thorHashToEth(transaction.ID()), nil
}

func (e *EthRPC) ethGetTransactionByHash(params []json.RawMessage) (any, *rpcError) {
	hashStr, err := paramString(params, 0)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	txID, err := thor.ParseBytes32(hashStr)
	if err != nil {
		return nil, invalidParams("invalid tx hash")
	}

	best := e.repo.BestBlockSummary().Header.ID()
	chain := e.repo.NewChain(best)

	t, meta, err := chain.GetTransaction(txID)
	if err != nil {
		// Check txpool for pending.
		if pending := e.pool.Get(txID); pending != nil && pending.Type() == tx.TypeEthTyped1559 {
			return convertTx(pending, nil, 0, e.chainID), nil
		}
		return nil, nil // not found → null
	}
	if t.Type() != tx.TypeEthTyped1559 {
		return nil, nil // VeChain-native → hidden
	}
	header, err := chain.GetBlockHeader(meta.BlockNum)
	if err != nil {
		return nil, internalErr(err)
	}
	return convertTx(t, header, uint64(meta.Index), e.chainID), nil
}

func (e *EthRPC) ethGetTransactionReceipt(params []json.RawMessage) (any, *rpcError) {
	hashStr, err := paramString(params, 0)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	txID, err := thor.ParseBytes32(hashStr)
	if err != nil {
		return nil, invalidParams("invalid tx hash")
	}

	best := e.repo.BestBlockSummary().Header.ID()
	chain := e.repo.NewChain(best)

	t, meta, err := chain.GetTransaction(txID)
	if err != nil {
		return nil, nil // not found or pending → null
	}
	if t.Type() != tx.TypeEthTyped1559 {
		return nil, nil // VeChain-native → hidden
	}

	receipt, err := chain.GetTransactionReceipt(txID)
	if err != nil {
		return nil, internalErr(err)
	}
	header, err := chain.GetBlockHeader(meta.BlockNum)
	if err != nil {
		return nil, internalErr(err)
	}
	return convertReceipt(receipt, t, header, uint64(meta.Index), e.chainID), nil
}

// --------------------------------------------------------------------------
// Logs
// --------------------------------------------------------------------------

func (e *EthRPC) ethGetLogs(params []json.RawMessage) (any, *rpcError) {
	if len(params) == 0 {
		return nil, invalidParams("missing filter object")
	}
	var filter logFilter
	if err := json.Unmarshal(params[0], &filter); err != nil {
		return nil, invalidParams(err.Error())
	}

	best := e.repo.BestBlockSummary().Header

	// Block range.
	var fromBlock, toBlock uint32
	if filter.BlockHash != nil {
		// If blockHash is specified, from/to are both that block.
		id := thor.Bytes32(*filter.BlockHash)
		sum, err := e.repo.GetBlockSummary(id)
		if err != nil {
			return []*rpcLog{}, nil
		}
		fromBlock = sum.Header.Number()
		toBlock = fromBlock
	} else {
		fromBlock = 0
		toBlock = best.Number()
		if filter.FromBlock != nil {
			h, err := e.resolveBlock(*filter.FromBlock)
			if err == nil {
				fromBlock = h.Number()
			}
		}
		if filter.ToBlock != nil {
			h, err := e.resolveBlock(*filter.ToBlock)
			if err == nil {
				toBlock = h.Number()
			}
		}
	}

	// Guard against huge ranges.
	const maxBlockRange = 2048
	if toBlock > fromBlock && toBlock-fromBlock > maxBlockRange {
		return nil, &rpcError{Code: codeInvalidParams, Message: fmt.Sprintf("block range exceeds maximum of %d blocks", maxBlockRange)}
	}

	// Build address filter set.
	addresses := parseAddresses(filter.Address)

	// Build criteria from topics.
	criteria, err := buildCriteria(addresses, filter.Topics)
	if err != nil {
		return nil, invalidParams(err.Error())
	}

	dbFilter := &logdb.EventFilter{
		CriteriaSet: criteria,
		Range:       &logdb.Range{From: fromBlock, To: toBlock},
		Options:     &logdb.Options{Limit: 10000},
		Order:       logdb.ASC,
	}

	events, err := e.logDB.FilterEvents(context.Background(), dbFilter)
	if err != nil {
		return nil, internalErr(err)
	}

	logs := make([]*rpcLog, len(events))
	for i, ev := range events {
		logs[i] = convertLogDBEvent(ev, uint64(ev.LogIndex))
	}
	return logs, nil
}

// parseAddresses extracts a list of thor addresses from the filter's address field,
// which can be a single hex string or an array of hex strings.
func parseAddresses(raw any) []thor.Address {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case string:
		return []thor.Address{ethAddrToThor(common.HexToAddress(v))}
	case []any:
		addrs := make([]thor.Address, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				addrs = append(addrs, ethAddrToThor(common.HexToAddress(s)))
			}
		}
		return addrs
	}
	return nil
}

// buildCriteria converts Ethereum topic filters to logdb EventCriteria.
// Ethereum topics are array-of-(string|array) where inner arrays mean OR.
// We expand these into multiple EventCriteria (one per OR combination).
func buildCriteria(addresses []thor.Address, topics []any) ([]*logdb.EventCriteria, error) {
	// Build per-position topic lists (each position is a list of acceptable hashes, nil = wildcard).
	type topicSet = []*thor.Bytes32 // nil entry means wildcard
	positions := make([][]topicSet, 0, 5)

	for _, topicEntry := range topics {
		if topicEntry == nil {
			// Wildcard: accept anything at this position.
			positions = append(positions, []topicSet{nil})
			continue
		}
		switch v := topicEntry.(type) {
		case string:
			h, err := thor.ParseBytes32(v)
			if err != nil {
				return nil, fmt.Errorf("invalid topic: %s", v)
			}
			positions = append(positions, []topicSet{{&h}})
		case []any:
			var opts []topicSet
			for _, item := range v {
				if item == nil {
					opts = append(opts, nil) // OR with wildcard
					continue
				}
				s, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("invalid topic type")
				}
				h, err := thor.ParseBytes32(s)
				if err != nil {
					return nil, fmt.Errorf("invalid topic: %s", s)
				}
				opts = append(opts, topicSet{&h})
			}
			positions = append(positions, opts)
		}
	}

	// Generate the cartesian product of OR options across positions.
	// Cap at 16 criteria to avoid combinatorial explosion.
	combos := [][5]*thor.Bytes32{{}}
	for i, opts := range positions {
		if i >= 5 {
			break
		}
		var next [][5]*thor.Bytes32
		for _, existing := range combos {
			for _, opt := range opts {
				c := existing
				if opt != nil {
					c[i] = opt[0]
				}
				next = append(next, c)
				if len(next) >= 16 {
					break
				}
			}
			if len(next) >= 16 {
				break
			}
		}
		combos = next
	}

	// Build final criteria, cross-producting addresses.
	if len(addresses) == 0 {
		addresses = []thor.Address{{}} // empty = no address filter
	}

	result := make([]*logdb.EventCriteria, 0, len(combos)*len(addresses))
	for _, combo := range combos {
		for _, addr := range addresses {
			c := &logdb.EventCriteria{Topics: combo}
			if addr != (thor.Address{}) {
				a := addr
				c.Address = &a
			}
			result = append(result, c)
		}
	}
	if len(result) == 0 {
		result = []*logdb.EventCriteria{{}} // no filter = return all events
	}
	return result, nil
}

// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"context"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/api/ethview"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

func init() {
	register("eth_getBlockByNumber", handleGetBlockByNumber)
	register("eth_getBlockByHash", handleGetBlockByHash)
	register("eth_getBlockTransactionCountByNumber", handleGetBlockTransactionCountByNumber)
	register("eth_getBlockTransactionCountByHash", handleGetBlockTransactionCountByHash)
}

// handleGetBlockByNumber resolves a tag to a canonical block and projects it.
// fullTx=true expands transactions into TransactionObject entries; any
// multi-clause tx causes the projection to fail with
// block_contains_tx_not_representable.
func handleGetBlockByNumber(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) != 2 {
		return nil, InvalidParams("expected [blockTag, fullTx]")
	}
	var tag BlockTag
	if err := json.Unmarshal(args[0], &tag); err != nil {
		return nil, InvalidParams("blockTag: " + err.Error())
	}
	var fullTx bool
	if err := json.Unmarshal(args[1], &fullTx); err != nil {
		return nil, InvalidParams("fullTx: " + err.Error())
	}

	_, summary, err := tag.Resolve(s.repo, s.bft)
	if err != nil {
		return nil, ToRPCError(err)
	}
	return projectBlockAt(s, summary.Header.ID(), fullTx)
}

// handleGetBlockByHash takes a bare 32-byte hash + fullTx flag.
func handleGetBlockByHash(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) != 2 {
		return nil, InvalidParams("expected [blockHash, fullTx]")
	}
	var hash thor.Bytes32
	if err := json.Unmarshal(args[0], &hash); err != nil {
		return nil, InvalidParams("blockHash: " + err.Error())
	}
	var fullTx bool
	if err := json.Unmarshal(args[1], &fullTx); err != nil {
		return nil, InvalidParams("fullTx: " + err.Error())
	}

	if _, err := s.repo.GetBlockSummary(hash); err != nil {
		if s.repo.IsNotFound(err) {
			return nil, nil
		}
		return nil, InternalError(err)
	}
	return projectBlockAt(s, hash, fullTx)
}

// handleGetBlockTransactionCountByNumber counts all txs in the block,
// including multi-clause ones (spec §5 "全量计数" — representability is
// irrelevant for counting).
func handleGetBlockTransactionCountByNumber(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) != 1 {
		return nil, InvalidParams("expected [blockTag]")
	}
	var tag BlockTag
	if err := json.Unmarshal(args[0], &tag); err != nil {
		return nil, InvalidParams("blockTag: " + err.Error())
	}
	_, summary, err := tag.Resolve(s.repo, s.bft)
	if err != nil {
		return nil, ToRPCError(err)
	}
	return countTxsInBlock(s, summary.Header.ID())
}

// handleGetBlockTransactionCountByHash is the hash-keyed variant.
func handleGetBlockTransactionCountByHash(_ context.Context, s *Server, params json.RawMessage) (any, *RPCError) {
	var args []json.RawMessage
	if err := json.Unmarshal(params, &args); err != nil || len(args) != 1 {
		return nil, InvalidParams("expected [blockHash]")
	}
	var hash thor.Bytes32
	if err := json.Unmarshal(args[0], &hash); err != nil {
		return nil, InvalidParams("blockHash: " + err.Error())
	}
	return countTxsInBlock(s, hash)
}

// --- helpers -------------------------------------------------------------

// projectBlockAt loads the block and drives ethview.ProjectBlock with the
// tx-meta callback the fullTx=true path needs.
func projectBlockAt(s *Server, blockID thor.Bytes32, fullTx bool) (any, *RPCError) {
	blk, err := s.repo.GetBlock(blockID)
	if err != nil {
		if s.repo.IsNotFound(err) {
			return nil, nil
		}
		return nil, InternalError(err)
	}
	view, err := ethview.ProjectBlock(blk, fullTx, func(idx int) ethview.TxMeta {
		return txMetaForBlockIndex(blk, idx)
	})
	if err != nil {
		return nil, FromEthViewError(err)
	}
	return view, nil
}

// txMetaForBlockIndex builds the per-tx ethview.TxMeta for a tx at a given
// index of blk. Origin recovery failure is swallowed — the tx would not have
// been mined if its signature didn't recover, so we treat a recover error as
// a defensive zero address.
func txMetaForBlockIndex(blk *block.Block, idx int) ethview.TxMeta {
	txs := blk.Transactions()
	if idx < 0 || idx >= len(txs) {
		return ethview.TxMeta{}
	}
	trx := txs[idx]

	blockID := blk.Header().ID()
	blockNum := blk.Header().Number()
	origin, _ := trx.Origin()
	delegator, _ := trx.Delegator()

	return ethview.TxMeta{
		BlockID:           &blockID,
		BlockNumber:       &blockNum,
		Index:             uint32(idx),
		Origin:            origin,
		Delegator:         delegator,
		EffectiveGasPrice: effectiveGasPriceForTx(trx, blk.Header().BaseFee()),
	}
}

// countTxsInBlock returns hex-encoded Transactions() length.
func countTxsInBlock(s *Server, blockID thor.Bytes32) (any, *RPCError) {
	blk, err := s.repo.GetBlock(blockID)
	if err != nil {
		if s.repo.IsNotFound(err) {
			return nil, nil
		}
		return nil, InternalError(err)
	}
	return hexutil.Uint64(len(blk.Transactions())), nil
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package proto

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const (
	// MaxBlockByIDResult is the maximum number of blocks that can be returned
	// by GetBlockByID. Since block IDs are unique, only 0 or 1 is valid.
	MaxBlockByIDResult = 1

	// MaxBlocksFromNumber is the maximum number of blocks that can be returned
	// by GetBlocksFromNumber. This matches the sender's limit in handle_rpc.go.
	MaxBlocksFromNumber = 1024
)

type (

	// Status result of MsgGetStatus.
	Status struct {
		GenesisBlockID thor.Bytes32
		SysTimestamp   uint64
		BestBlockID    thor.Bytes32
		TotalScore     uint64
	}
)

// RPC defines RPC interface.
type RPC interface {
	Notify(ctx context.Context, msgCode uint64, arg any) error
	Call(ctx context.Context, msgCode uint64, arg any, result any, maxResultSize uint32) error
}

// decodeRawListLimited streams an RLP list of raw values, stopping as soon as
// limit is exceeded so that a malicious peer cannot make us materialize a huge
// list before the count is checked.
func decodeRawListLimited(s *rlp.Stream, limit int) ([]rlp.RawValue, error) {
	if _, err := s.List(); err != nil {
		return nil, err
	}
	var raws []rlp.RawValue
	for {
		var raw rlp.RawValue
		err := s.Decode(&raw)
		if err == rlp.EOL {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(raws) == limit {
			return nil, fmt.Errorf("result size exceeds limit: > %d", limit)
		}
		raws = append(raws, raw)
	}
	if err := s.ListEnd(); err != nil {
		return nil, err
	}
	return raws, nil
}

// blockByIDResult is the decode target for MsgGetBlockByID.
type blockByIDResult []rlp.RawValue

func (r *blockByIDResult) DecodeRLP(s *rlp.Stream) error {
	raws, err := decodeRawListLimited(s, MaxBlockByIDResult)
	if err != nil {
		return err
	}
	*r = raws
	return nil
}

// blocksFromNumberResult is the decode target for MsgGetBlocksFromNumber.
type blocksFromNumberResult []rlp.RawValue

func (r *blocksFromNumberResult) DecodeRLP(s *rlp.Stream) error {
	raws, err := decodeRawListLimited(s, MaxBlocksFromNumber)
	if err != nil {
		return err
	}
	*r = raws
	return nil
}

// GetStatus get status of remote peer.
func GetStatus(ctx context.Context, rpc RPC) (*Status, error) {
	var status Status
	if err := rpc.Call(ctx, MsgGetStatus, &struct{}{}, &status, noResultSizeLimit); err != nil {
		return nil, err
	}
	return &status, nil
}

// NotifyNewBlockID notify new block ID to remote peer.
func NotifyNewBlockID(ctx context.Context, rpc RPC, id thor.Bytes32) error {
	return rpc.Notify(ctx, MsgNewBlockID, &id)
}

// NotifyNewBlock notify new block to remote peer.
func NotifyNewBlock(ctx context.Context, rpc RPC, block *block.Block) error {
	return rpc.Notify(ctx, MsgNewBlock, block)
}

// NotifyNewTx notify new tx to remote peer.
func NotifyNewTx(ctx context.Context, rpc RPC, tx *tx.Transaction) error {
	return rpc.Notify(ctx, MsgNewTx, tx)
}

// GetBlockByID query block from remote peer by given block ID.
// It may return nil block even no error.
func GetBlockByID(ctx context.Context, rpc RPC, id thor.Bytes32) (rlp.RawValue, error) {
	var result blockByIDResult
	if err := rpc.Call(ctx, MsgGetBlockByID, id, &result, noResultSizeLimit); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result[0], nil
}

// GetBlockIDByNumber query block ID from remote peer by given number.
func GetBlockIDByNumber(ctx context.Context, rpc RPC, num uint32) (thor.Bytes32, error) {
	var id thor.Bytes32
	if err := rpc.Call(ctx, MsgGetBlockIDByNumber, num, &id, noResultSizeLimit); err != nil {
		return thor.Bytes32{}, err
	}
	return id, nil
}

// GetBlocksFromNumber get a batch of blocks starts with num from remote peer.
func GetBlocksFromNumber(ctx context.Context, rpc RPC, num uint32) ([]rlp.RawValue, error) {
	var blocks blocksFromNumberResult
	if err := rpc.Call(ctx, MsgGetBlocksFromNumber, num, &blocks, noResultSizeLimit); err != nil {
		return nil, err
	}
	return blocks, nil
}

// GetTxs get txs from remote peer.
func GetTxs(ctx context.Context, rpc RPC) (tx.Transactions, error) {
	var txs tx.Transactions
	if err := rpc.Call(ctx, MsgGetTxs, &struct{}{}, &txs, MaxGetTxsResultSize); err != nil {
		return nil, err
	}
	return txs, nil
}

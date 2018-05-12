package proto

import (
	"context"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type (
	// Status arg of MsgStatus.
	Status struct{}

	// StatusResult result of MsgStatus.
	StatusResult struct {
		GenesisBlockID thor.Bytes32
		SysTimestamp   uint64
		BestBlockID    thor.Bytes32
		TotalScore     uint64
	}

	// NewBlockID arg of MsgNewBlockID.
	NewBlockID struct{ ID thor.Bytes32 }

	// NewBlock arg of MsgNewBlock.
	NewBlock struct{ Block *block.Block }

	// NewTx arg of MsgNewTx
	NewTx struct{ Tx *tx.Transaction }

	// GetBlockIDByNumber arg of MsgGetBlockIDByNumber.
	GetBlockIDByNumber struct{ Num uint32 }

	// GetBlockIDByNumberResult result of MsgGetBlockIDByNumber.
	GetBlockIDByNumberResult struct{ ID thor.Bytes32 }

	// GetBlocksFromNumber arg of MsgGetBlocksFromNumber.
	GetBlocksFromNumber struct{ Num uint32 }

	// GetBlocksFromNumberResult result of MsgGetBlocksFromNumber.
	GetBlocksFromNumberResult []rlp.RawValue

	// GetBlockByID arg of MsgGetBlockByID.
	GetBlockByID struct{ ID thor.Bytes32 }

	// GetBlockByIDResult result of MsgGetBlockByID.
	GetBlockByIDResult struct {
		Block *block.Block `rlp:"nil"`
	}

	// GetTxs arg of MsgGetTxs.
	GetTxs struct{}

	// GetTxsResult result of MsgGetTxs.
	GetTxsResult []*tx.Transaction
)

// RPC defines RPC interface.
type RPC interface {
	Call(ctx context.Context, msgCode uint64, arg interface{}, result interface{}) error
}

// Call perform RPC call.
func (a Status) Call(ctx context.Context, rpc RPC) (*StatusResult, error) {
	var result StatusResult
	if err := rpc.Call(ctx, MsgStatus, &a, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Call perform RPC call.
func (a NewBlockID) Call(ctx context.Context, rpc RPC) error {
	return rpc.Call(ctx, MsgNewBlockID, &a, &struct{}{})
}

// Call perform RPC call.
func (a NewBlock) Call(ctx context.Context, rpc RPC) error {
	return rpc.Call(ctx, MsgNewBlock, &a, &struct{}{})
}

// Call perform RPC call.
func (a NewTx) Call(ctx context.Context, rpc RPC) error {
	return rpc.Call(ctx, MsgNewTx, &a, &struct{}{})
}

// Call perform RPC call.
func (a GetBlockIDByNumber) Call(ctx context.Context, rpc RPC) (*GetBlockIDByNumberResult, error) {
	var result GetBlockIDByNumberResult
	if err := rpc.Call(ctx, MsgGetBlockIDByNumber, &a, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Call perform RPC call.
func (a GetBlocksFromNumber) Call(ctx context.Context, rpc RPC) (GetBlocksFromNumberResult, error) {
	var result GetBlocksFromNumberResult
	if err := rpc.Call(ctx, MsgGetBlocksFromNumber, &a, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Call perform RPC call.
func (a GetBlockByID) Call(ctx context.Context, rpc RPC) (*GetBlockByIDResult, error) {
	var result GetBlockByIDResult
	if err := rpc.Call(ctx, MsgGetBlockByID, &a, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Call perform RPC call.
func (a GetTxs) Call(ctx context.Context, rpc RPC) (GetTxsResult, error) {
	var result GetTxsResult
	if err := rpc.Call(ctx, MsgGetTxs, &a, &result); err != nil {
		return nil, err
	}
	return result, nil
}

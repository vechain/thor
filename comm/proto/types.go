package proto

import (
	"context"
	"io"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type (

	// Status result of MsgGetStatus.
	Status struct {
		GenesisBlockID thor.Bytes32
		SysTimestamp   uint64
		BestBlockID    thor.Bytes32
		TotalScore     uint64
	}

	nilableBlock struct{ *block.Block }
)

// RPC defines RPC interface.
type RPC interface {
	Notify(ctx context.Context, msgCode uint64, arg interface{}) error
	Call(ctx context.Context, msgCode uint64, arg interface{}, result interface{}) error
}

// GetStatus get status of remote peer.
func GetStatus(ctx context.Context, rpc RPC) (*Status, error) {
	var status Status
	if err := rpc.Call(ctx, MsgGetStatus, &struct{}{}, &status); err != nil {
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
func GetBlockByID(ctx context.Context, rpc RPC, id thor.Bytes32) (*block.Block, error) {
	var result nilableBlock
	if err := rpc.Call(ctx, MsgGetBlockByID, id, &result); err != nil {
		return nil, err
	}
	return result.Block, nil
}

// GetBlockIDByNumber query block ID from remote peer by given number.
func GetBlockIDByNumber(ctx context.Context, rpc RPC, num uint32) (thor.Bytes32, error) {
	var id thor.Bytes32
	if err := rpc.Call(ctx, MsgGetBlockIDByNumber, num, &id); err != nil {
		return thor.Bytes32{}, err
	}
	return id, nil
}

// GetBlocksFromNumber get a batch of blocks starts with num from remote peer.
func GetBlocksFromNumber(ctx context.Context, rpc RPC, num uint32) ([]rlp.RawValue, error) {
	var blocks []rlp.RawValue
	if err := rpc.Call(ctx, MsgGetBlocksFromNumber, num, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

// GetTxs get txs from remote peer.
func GetTxs(ctx context.Context, rpc RPC) (tx.Transactions, error) {
	var txs tx.Transactions
	if err := rpc.Call(ctx, MsgGetTxs, &struct{}{}, &txs); err != nil {
		return nil, err
	}
	return txs, nil
}

// EncodeRLP implements rlp.Encoder.
func (m *nilableBlock) EncodeRLP(w io.Writer) error {
	if m.Block == nil {
		return rlp.Encode(w, &struct{}{})
	}
	return rlp.Encode(w, m.Block)
}

// DecodeRLP implements rlp.Decoder.
func (m *nilableBlock) DecodeRLP(s *rlp.Stream) error {
	kind, size, err := s.Kind()
	if err != nil {
		return err
	}
	if kind != rlp.List {
		return rlp.ErrExpectedList
	}
	if size > 0 {
		return s.Decode(&m.Block)
	}
	if err := s.Decode(&struct{}{}); err != nil {
		return err
	}
	m.Block = nil
	return nil
}

var _ rlp.Encoder = (*nilableBlock)(nil)
var _ rlp.Decoder = (*nilableBlock)(nil)

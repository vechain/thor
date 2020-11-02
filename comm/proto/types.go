// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package proto

import (
	"context"
	"encoding/binary"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Status result of MsgGetStatus.
type Status struct {
	GenesisBlockID thor.Bytes32
	SysTimestamp   uint64
	BestBlockID    thor.Bytes32
	TotalScore     uint64
}

// Accepted is the backer's signature to an proposal with proposal's hash.
type Accepted struct {
	ProposalHash thor.Bytes32
	Signature    block.ComplexSignature
}

// Hash computes the hash of accepted.
func (acc *Accepted) Hash() thor.Bytes32 {
	return thor.Blake2b(acc.ProposalHash.Bytes(), ([]byte)(acc.Signature))
}

// Draft is constructed by the leader's proposal and signature.
type Draft struct {
	Proposal  *block.Proposal
	Signature []byte
}

// Hash computes the hash of draft.
func (d *Draft) Hash() thor.Bytes32 {
	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b, d.Proposal.GasLimit)
	binary.BigEndian.PutUint64(b[8:], d.Proposal.Timestamp)

	// [parentID + txsRoot + Gaslimit + Timestamp + Signature]
	return thor.Blake2b(d.Proposal.ParentID.Bytes(), d.Proposal.TxsRoot.Bytes(), b, d.Signature)
}

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

// NotifyNewDraft notify a draft to remote peer.
func NotifyNewDraft(ctx context.Context, rpc RPC, d *Draft) error {
	return rpc.Notify(ctx, MsgNewDraft, d)
}

// NotifyNewAccepted notify an accepted message` to remote peer.
func NotifyNewAccepted(ctx context.Context, rpc RPC, acc *Accepted) error {
	return rpc.Notify(ctx, MsgNewAccepted, acc)
}

// GetBlockByID query block from remote peer by given block ID.
// It may return nil block even no error.
func GetBlockByID(ctx context.Context, rpc RPC, id thor.Bytes32) (rlp.RawValue, error) {
	var result []rlp.RawValue
	if err := rpc.Call(ctx, MsgGetBlockByID, id, &result); err != nil {
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

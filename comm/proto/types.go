// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package proto

import (
	"context"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

type (

	// Status result of MsgGetStatus.
	Status struct {
		GenesisBlockID thor.Bytes32
		SysTimestamp   uint64
		BestBlockID    thor.Bytes32
		TotalScore     uint64
	}

	// VRFProof represents a VRF proof from a validator
	VRFProof struct {
		ValidatorAddress thor.Address
		Alpha            []byte // seed for VRF
		Proof            []byte // VRF proof
		BlockNumber      uint32 // block number this proof is for
		Timestamp        uint64 // when the proof was generated
	}

	// VRFProofRequest represents a request for VRF proofs
	VRFProofRequest struct {
		Alpha       []byte         // seed for VRF
		BlockNumber uint32         // block number we need proofs for
		Validators  []thor.Address // list of validators we need proofs from
	}
)

// RPC defines RPC interface.
type RPC interface {
	Notify(ctx context.Context, msgCode uint64, arg any) error
	Call(ctx context.Context, msgCode uint64, arg any, result any) error
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

// GetTxs get transactions from remote peer.
func GetTxs(ctx context.Context, rpc RPC) (tx.Transactions, error) {
	var result tx.Transactions
	if err := rpc.Call(ctx, MsgGetTxs, &struct{}{}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// NotifyVRFProof notify a VRF proof to remote peer
func NotifyVRFProof(ctx context.Context, rpc RPC, proof *VRFProof) error {
	return rpc.Notify(ctx, MsgVRFProof, proof)
}

// RequestVRFProofs request VRF proofs from remote peer
func RequestVRFProofs(ctx context.Context, rpc RPC, request *VRFProofRequest) ([]*VRFProof, error) {
	var proofs []*VRFProof
	if err := rpc.Call(ctx, MsgVRFProofRequest, request, &proofs); err != nil {
		return nil, err
	}
	return proofs, nil
}

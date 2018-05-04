package proto

import (
	"context"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// ReqStatus request payload of MsgStatus.
type ReqStatus struct{}

// Do make request to peer.
func (req ReqStatus) Do(ctx context.Context, peer *p2psrv.Peer) (*RespStatus, error) {
	var resp RespStatus
	if err := peer.Request(ctx, MsgStatus, &req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RespStatus response payload of MsgStatus.
type RespStatus struct {
	GenesisBlockID thor.Bytes32
	SysTimestamp   uint64
	BestBlockID    thor.Bytes32
	TotalScore     uint64
}

// ReqNewBlockID request payload of MsgNewBlockID.
type ReqNewBlockID struct {
	ID thor.Bytes32
}

// Do make request to peer.
func (req ReqNewBlockID) Do(ctx context.Context, peer *p2psrv.Peer) error {
	var resp struct{}
	return peer.Request(ctx, MsgNewBlockID, &req, &resp)
}

// ReqMsgNewTx request payload of MsgNewTx.
type ReqMsgNewTx struct {
	Tx *tx.Transaction
}

// Do make request to peer.
func (req ReqMsgNewTx) Do(ctx context.Context, peer *p2psrv.Peer) error {
	var resp struct{}
	return peer.Request(ctx, MsgNewTx, &req, &resp)
}

// ReqNewBlock request payload of MsgNewBlock.
type ReqNewBlock struct {
	Block *block.Block
}

// Do make request.
func (req ReqNewBlock) Do(ctx context.Context, peer *p2psrv.Peer) error {
	var resp struct{}
	return peer.Request(ctx, MsgNewBlock, &req, &resp)
}

// ReqGetBlockIDByNumber request payload of MsgGetBlockIDByNumber.
type ReqGetBlockIDByNumber struct {
	Num uint32
}

// Do make request to peer.
func (req ReqGetBlockIDByNumber) Do(ctx context.Context, peer *p2psrv.Peer) (*RespGetBlockIDByNumber, error) {
	var resp RespGetBlockIDByNumber
	if err := peer.Request(ctx, MsgGetBlockIDByNumber, &req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RespGetBlockIDByNumber response payload of MsgGetBlockIDByNumber.
type RespGetBlockIDByNumber struct {
	ID thor.Bytes32
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ReqGetBlocksFromNumber request payload of MsgGetBlocksFromNumber.
type ReqGetBlocksFromNumber struct {
	Num uint32
}

// Do make request to peer.
func (req ReqGetBlocksFromNumber) Do(ctx context.Context, peer *p2psrv.Peer) (RespGetBlocksFromNumber, error) {
	var resp RespGetBlocksFromNumber
	if err := peer.Request(ctx, MsgGetBlocksFromNumber, &req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// RespGetBlocksFromNumber response payload of MsgGetBlocksByNumber.
type RespGetBlocksFromNumber []rlp.RawValue

////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ReqGetBlockByID request payload of MsgGetBlockByID.
type ReqGetBlockByID struct {
	ID thor.Bytes32
}

// Do make request to peer.
func (req ReqGetBlockByID) Do(ctx context.Context, peer *p2psrv.Peer) (*RespGetBlockByID, error) {
	var resp RespGetBlockByID
	if err := peer.Request(ctx, MsgGetBlockByID, &req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RespGetBlockByID response payload of MsgGetBlockByID.
type RespGetBlockByID struct {
	Block *block.Block
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ReqGetTxs request payload of MsgGetTxs.
type ReqGetTxs struct{}

// Do make request to peer.
func (req ReqGetTxs) Do(ctx context.Context, peer *p2psrv.Peer) (RespGetTxs, error) {
	var resp RespGetTxs
	if err := peer.Request(ctx, MsgGetTxs, &req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// RespGetTxs response payload of MsgGetTxs.
type RespGetTxs []*tx.Transaction

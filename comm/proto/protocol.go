package proto

import (
	"context"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Constants
const (
	Name              = "thor"
	Version    uint32 = 1
	Length     uint64 = 40
	MaxMsgSize        = 10 * 1024 * 1024
)

// Protocol messages of thor
const (
	MsgStatus = iota
	MsgNewBlockID
	MsgNewBlock
	MsgNewTx
	MsgGetBlockByID
	MsgGetBlockIDByNumber
	MsgGetBlocksByNumber // 获取某个 Num 之后的部分块 (不包含 num 所在的块)
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
	GenesisBlockID thor.Hash
	BestBlockID    thor.Hash
	TotalScore     uint64
}

// ReqNewBlockID request payload of MsgNewBlockID.
type ReqNewBlockID struct {
	ID thor.Hash
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
	ID thor.Hash
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ReqGetBlocksByNumber request payload of MsgGetBlocksByNumber.
type ReqGetBlocksByNumber struct {
	Num uint32
}

// Do make request to peer.
func (req ReqGetBlocksByNumber) Do(ctx context.Context, peer *p2psrv.Peer) (RespGetBlocksByNumber, error) {
	var resp RespGetBlocksByNumber
	if err := peer.Request(ctx, MsgGetBlocksByNumber, &req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// RespGetBlocksByNumber response payload of MsgGetBlocksByNumber.
type RespGetBlocksByNumber []*block.Block

////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ReqGetBlockByID request payload of MsgGetBlockByID.
type ReqGetBlockByID struct {
	ID thor.Hash
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

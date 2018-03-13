package proto

import (
	"context"

	"github.com/vechain/thor/block"

	"github.com/vechain/thor/p2psrv"
	"github.com/vechain/thor/thor"
)

// Constants
const (
	Name              = "thor"
	Version    uint32 = 1
	Length     uint64 = 4
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

// RespStatus response payload of MsgStatus.
type RespStatus struct {
	Session     *p2psrv.Session
	BestBlockID thor.Hash
	TotalScore  uint64
}

// ReqStatus request payload of MsgStatus.
type ReqStatus struct{}

// Do make request to session.
func (req ReqStatus) Do(ctx context.Context, session *p2psrv.Session) (*RespStatus, error) {
	var resp RespStatus
	if err := session.Request(ctx, MsgStatus, &req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ReqNewBlockID request payload of MsgNewBlockID.
type ReqNewBlockID thor.Hash

// Do make request to session.
func (req ReqNewBlockID) Do(ctx context.Context, session *p2psrv.Session) error {
	var resp struct{}
	return session.Request(ctx, MsgNewBlockID, &req, &resp)
}

// ReqNewBlock request payload of MsgNewBlock.
type ReqNewBlock struct {
	Block *block.Block
}

// Do make request.
func (req ReqNewBlock) Do(ctx context.Context, session *p2psrv.Session) error {
	var resp struct{}
	return session.Request(ctx, MsgNewBlock, &req, &resp)
}

// ReqGetBlockIDByNumber request payload of MsgGetBlockIDByNumber.
type ReqGetBlockIDByNumber uint32

// Do make request to session.
func (req ReqGetBlockIDByNumber) Do(ctx context.Context, session *p2psrv.Session) (*RespGetBlockIDByNumber, error) {
	var resp RespGetBlockIDByNumber
	if err := session.Request(ctx, MsgGetBlockIDByNumber, &req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RespGetBlockIDByNumber response payload of MsgGetBlockIDByNumber.
type RespGetBlockIDByNumber thor.Hash

////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ReqGetBlocksByNumber request payload of MsgGetBlocksByNumber.
type ReqGetBlocksByNumber uint32

// Do make request to session.
func (req ReqGetBlocksByNumber) Do(ctx context.Context, session *p2psrv.Session) (RespGetBlocksByNumber, error) {
	var resp RespGetBlocksByNumber
	if err := session.Request(ctx, MsgGetBlockIDByNumber, &req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// RespGetBlocksByNumber response payload of MsgGetBlocksByNumber.
type RespGetBlocksByNumber []*block.Block

////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ReqGetBlockByID request payload of MsgGetBlockByID.
type ReqGetBlockByID thor.Hash

// Do make request to session.
func (req ReqGetBlockByID) Do(ctx context.Context, session *p2psrv.Session) (*RespGetBlockByID, error) {
	var resp RespGetBlockByID
	if err := session.Request(ctx, MsgGetBlockByID, &req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RespGetBlockByID response payload of MsgGetBlockByID.
type RespGetBlockByID block.Block

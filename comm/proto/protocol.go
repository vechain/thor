package proto

import (
	"fmt"
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
	MsgGetBlocksFromNumber // fetch blocks from given number (including given number)
)

// MsgName convert msg code to string.
func MsgName(msgCode uint64) string {
	switch msgCode {
	case MsgStatus:
		return "MsgStatus"
	case MsgNewBlockID:
		return "MsgNewBlockID"
	case MsgNewBlock:
		return "MsgNewBlock"
	case MsgNewTx:
		return "MsgNewTx"
	case MsgGetBlockByID:
		return "MsgGetBlockByID"
	case MsgGetBlockIDByNumber:
		return "MsgGetBlockIDByNumber"
	case MsgGetBlocksFromNumber:
		return "MsgGetBlocksFromNumber"
	default:
		return fmt.Sprintf("unknown msg code(%v)", msgCode)
	}
}

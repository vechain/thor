// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package proto

import (
	"fmt"
)

// Constants
const (
	Name              = "thor"
	Version    uint   = 1
	Length     uint64 = 8
	MaxMsgSize        = 10 * 1024 * 1024
)

// Protocol messages of thor
const (
	MsgGetStatus = iota
	MsgNewBlockID
	MsgNewBlock
	MsgNewTx
	MsgGetBlockByID
	MsgGetBlockIDByNumber
	MsgGetBlocksFromNumber // fetch blocks from given number (including given number)
	MsgGetTxs
)

// MsgName convert msg code to string.
func MsgName(msgCode uint64) string {
	switch msgCode {
	case MsgGetStatus:
		return "MsgGetStatus"
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
	case MsgGetTxs:
		return "MsgGetTxs"
	default:
		return fmt.Sprintf("unknown msg code(%v)", msgCode)
	}
}

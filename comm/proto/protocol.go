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

	// MaxGetTxsResultSize bounds the MsgGetTxs response before it is decoded.
	// The sender caps a batch at maxTxSyncSize (100 KB, see comm/handle_rpc.go)
	// and a single tx at txpool.MaxTxSize (64 KB), so 256 KB covers the last tx
	// crossing the batch boundary. tx.Transactions is the only unbounded decode
	// target among the responses, and so the only one that needs narrowing.
	MaxGetTxsResultSize = 256 * 1024

	// noResultSizeLimit opts out of per-call narrowing, leaving the response
	// bounded only by MaxMsgSize in rpc.Serve. Used for fixed-width decode
	// targets, and where no bound tighter than MaxMsgSize can be derived.
	noResultSizeLimit = 0
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

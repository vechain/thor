// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import "github.com/vechain/thor/v2/p2p"

type msgData struct {
	ID       uint32
	IsResult bool
	Payload  any
}

type resultListener struct {
	msgCode  uint64
	onResult func(*p2p.Msg) error
}

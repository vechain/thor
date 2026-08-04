// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package dto

import (
	"github.com/vechain/thor/v2/thor"
)

type TxPoolStatus struct {
	Amount uint `json:"amount"`
}

type PeerStats struct {
	Name        string       `json:"name"`
	BestBlockID thor.Bytes32 `json:"bestBlockID"`
	TotalScore  uint64       `json:"totalScore"`
	PeerID      string       `json:"peerID"`
	NetAddr     string       `json:"netAddr"`
	Inbound     bool         `json:"inbound"`
	Duration    uint64       `json:"duration"`
}

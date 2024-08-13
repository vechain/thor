// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/thor"
)

type Network interface {
	PeersStats() []*comm.PeerStats
}

type Info struct {
	Version           string `json:"version"`
	LogsEnabled       bool   `json:"logsEnabled"`
	LogsLimit         uint64 `json:"logsLimit"`
	DebugEnabled      bool   `json:"debugEnabled"`
	DebugCustomTracer bool   `json:"debugCustomTracer"`
	CallGasLimit      uint64 `json:"callGasLimit"`
	WSBackTraceLimit  uint32 `json:"wsBackTraceLimit"`
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

func ConvertPeersStats(ss []*comm.PeerStats) []*PeerStats {
	if len(ss) == 0 {
		return nil
	}
	peersStats := make([]*PeerStats, len(ss))
	for i, peerStats := range ss {
		peersStats[i] = &PeerStats{
			Name:        peerStats.Name,
			BestBlockID: peerStats.BestBlockID,
			TotalScore:  peerStats.TotalScore,
			PeerID:      peerStats.PeerID,
			NetAddr:     peerStats.NetAddr,
			Inbound:     peerStats.Inbound,
			Duration:    peerStats.Duration,
		}
	}
	return peersStats
}

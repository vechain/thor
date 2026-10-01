// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package convert

import (
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/comm"
)

func ConvertPeersStats(ss []*comm.PeerStats) []*api.PeerStats {
	if len(ss) == 0 {
		return nil
	}
	peersStats := make([]*api.PeerStats, len(ss))
	for i, peerStats := range ss {
		peersStats[i] = &api.PeerStats{
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

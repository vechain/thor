package node

import (
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/thor"
)

type Network interface {
	PeersStats() []*comm.PeerStats
}

type PeerStats struct {
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

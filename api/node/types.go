package node

import (
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/thor"
)

type Network interface {
	SessionsStats() []*comm.SessionStats
}

type SessionStats struct {
	BestBlockID thor.Bytes32 `json:"bestBlockID"`
	TotalScore  uint64       `json:"totalScore"`
	PeerID      string       `json:"peerID"`
	NetAddr     string       `json:"netAddr"`
	Inbound     bool         `json:"inbound"`
	Duration    uint64       `json:"duration"`
}

func ConvertSessionStats(ss []*comm.SessionStats) []*SessionStats {
	if len(ss) == 0 {
		return nil
	}
	sessionStats := make([]*SessionStats, len(ss))
	for i, sessionStat := range ss {
		sessionStats[i] = &SessionStats{
			BestBlockID: sessionStat.BestBlockID,
			TotalScore:  sessionStat.TotalScore,
			PeerID:      sessionStat.PeerID,
			NetAddr:     sessionStat.NetAddr,
			Inbound:     sessionStat.Inbound,
			Duration:    sessionStat.Duration,
		}
	}
	return sessionStats
}

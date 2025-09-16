package main

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/p2p/discv5"

	"github.com/vechain/thor/v2/metrics"
)

var metricsPeerCount = metrics.GaugeVec("disco_peercount", []string{"id"})

func pollMetrics(ctx context.Context, net *discv5.Network) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nodes := make([]*discv5.Node, 1000)
			net.ReadRandomNodes(nodes)
			var read int64
			for _, n := range nodes {
				if n == nil {
					break
				}
				read++
			}
			metricsPeerCount.SetWithLabel(read, map[string]string{"id": net.Self().ID.String()})
		}
	}
}

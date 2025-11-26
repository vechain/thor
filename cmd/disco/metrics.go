// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"context"
	"time"

	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/p2p/tempdiscv5"
)

var metricsPeerCount = metrics.GaugeVec("disco_peercount", []string{"id"})

func pollMetrics(ctx context.Context, net *tempdiscv5.Network) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nodes := make([]*tempdiscv5.Node, 1000)
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

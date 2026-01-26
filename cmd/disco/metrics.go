// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/metrics"
	"github.com/vechain/thor/v2/p2p/discv5/discover"
	"github.com/vechain/thor/v2/p2p/tempdiscv5"
)

var metricsPeerCount = metrics.LazyLoadGaugeVec("disco_peercount", []string{"id", "network"})

// startMetricsServer starts an HTTP server that exposes metrics at /metrics endpoint.
// Returns the metrics URL, a cleanup function, and any error encountered.
func startMetricsServer(addr string) (string, func(), error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, errors.Wrapf(err, "listen metrics API addr [%v]", addr)
	}

	router := mux.NewRouter()
	router.PathPrefix("/metrics").Handler(metrics.HTTPHandler())
	handler := handlers.CompressHandler(router)

	srv := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Serve(listener)
	}()

	return "http://" + listener.Addr().String() + "/metrics", func() {
		srv.Close()
		wg.Wait()
	}, nil
}

// pollMetrics periodically collects and reports metrics from both discovery networks.
func pollMetrics(ctx context.Context, discv5 *discover.UDPv5, tempnet *tempdiscv5.Network) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Collect metrics from discv5 network
			if discv5 != nil {
				nodes := discv5.AllNodes()
				localNode := discv5.LocalNode()
				metricsPeerCount().SetWithLabel(int64(len(nodes)), map[string]string{
					"id":      localNode.ID().String(),
					"network": "discv5",
				})
			}

			// Collect metrics from tempdiscv5 network
			if tempnet != nil {
				nodes := make([]*tempdiscv5.Node, 1000)
				count := tempnet.ReadRandomNodes(nodes)
				localNode := tempnet.Self()
				metricsPeerCount().SetWithLabel(int64(count), map[string]string{
					"id":      localNode.ID.String(),
					"network": "tempdiscv5",
				})
			}
		}
	}
}

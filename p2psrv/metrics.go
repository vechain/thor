package p2psrv

import "github.com/vechain/thor/v2/telemetry"

var (
	metricConnectedPeers = telemetry.LazyLoad(func() telemetry.GaugeMeter {
		return telemetry.Gauge("p2p_connected_peers_count")
	})
	metricDiscoveredNodes = telemetry.LazyLoad(func() telemetry.GaugeMeter {
		return telemetry.Gauge("p2p_discovered_node_count")
	})
	metricDialNewNode = telemetry.LazyLoad(func() telemetry.CountMeter {
		return telemetry.Counter("p2p_dial_new_node_count")
	})
)

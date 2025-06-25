// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
	"time"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/thor"
)

type Health struct {
	repo *chain.Repository
	p2p  *comm.Communicator
}

const (
	defaultBlockTolerance = time.Duration(2*thor.BlockInterval) * time.Second // 2 blocks tolerance
	defaultMinPeerCount   = 2
)

func New(repo *chain.Repository, p2p *comm.Communicator) *Health {
	return &Health{
		repo: repo,
		p2p:  p2p,
	}
}

// isNetworkProgressing checks if the network is producing new blocks within the allowed interval.
func (h *Health) isNetworkProgressing(now time.Time, bestBlockTimestamp time.Time, blockTolerance time.Duration) bool {
	return now.Sub(bestBlockTimestamp) <= blockTolerance
}

// isNodeConnectedP2P checks if the node is connected to peers
func (h *Health) isNodeConnectedP2P(peerCount int, minPeerCount int) bool {
	return peerCount >= minPeerCount
}

func (h *Health) Status(blockTolerance time.Duration, minPeerCount int) (*api.AdminHealthStatus, error) {
	// Fetch the best block details
	bestBlock := h.repo.BestBlockSummary()
	bestBlockTimestamp := time.Unix(int64(bestBlock.Header.Timestamp()), 0)

	// Fetch the current connected peers
	var connectedPeerCount int
	if h.p2p == nil {
		connectedPeerCount = minPeerCount // ignore peers in solo mode
	} else {
		connectedPeerCount = h.p2p.PeerCount()
	}

	now := time.Now()

	// Perform the checks
	networkProgressing := h.isNetworkProgressing(now, bestBlockTimestamp, blockTolerance)
	nodeConnected := h.isNodeConnectedP2P(connectedPeerCount, minPeerCount)

	// Calculate overall health status
	healthy := networkProgressing && nodeConnected

	// Return the current status
	return &api.AdminHealthStatus{
		Healthy:              healthy,
		BestBlockTime:        &bestBlockTimestamp,
		IsNetworkProgressing: networkProgressing,
		PeerCount:            connectedPeerCount,
	}, nil
}

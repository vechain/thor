// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
	"sync"
	"time"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/thor"
)

type BlockIngestion struct {
	ID        *thor.Bytes32 `json:"id"`
	Timestamp *time.Time    `json:"timestamp"`
}

type Status struct {
	Healthy           bool            `json:"healthy"`
	BlockIngestion    *BlockIngestion `json:"blockIngestion"`
	ChainBootstrapped bool            `json:"chainBootstrapped"`
	PeerCount         int             `json:"peerCount"`
}

type Health struct {
	lock               sync.RWMutex
	repo               *chain.Repository
	p2p                *comm.Communicator
	isNodeBootstrapped bool
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

// hasNodeBootstrapped checks if the node has bootstrapped by comparing the block interval.
// Once it's marked as done, it never reverts.
func (h *Health) hasNodeBootstrapped(now time.Time, bestBlockTimestamp time.Time) bool {
	if h.isNodeBootstrapped {
		return true
	}

	blockInterval := time.Duration(thor.BlockInterval) * time.Second
	if bestBlockTimestamp.Add(blockInterval).After(now) {
		h.isNodeBootstrapped = true
	}

	return h.isNodeBootstrapped
}

// isNodeConnectedP2P checks if the node is connected to peers
func (h *Health) isNodeConnectedP2P(peerCount int, minPeerCount int) bool {
	return peerCount >= minPeerCount
}

func (h *Health) Status(blockTolerance time.Duration, minPeerCount int) (*Status, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	// Fetch the best block details
	bestBlock := h.repo.BestBlockSummary()
	bestBlockID := bestBlock.Header.ID()
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
	nodeBootstrapped := h.hasNodeBootstrapped(now, bestBlockTimestamp)
	nodeConnected := h.isNodeConnectedP2P(connectedPeerCount, minPeerCount)

	// Calculate overall health status
	healthy := networkProgressing && nodeBootstrapped && nodeConnected

	// Return the current status
	return &Status{
		Healthy: healthy,
		BlockIngestion: &BlockIngestion{
			ID:        &bestBlockID,
			Timestamp: &bestBlockTimestamp,
		},
		ChainBootstrapped: nodeBootstrapped,
		PeerCount:         connectedPeerCount,
	}, nil
}

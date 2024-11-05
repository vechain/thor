// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
	"sync"
	"time"

	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/chain"

	"github.com/vechain/thor/v2/thor"
)

type BlockIngestion struct {
	ID        thor.Bytes32 `json:"id"`
	Timestamp *time.Time   `json:"timestamp"`
}

type Status struct {
	Healthy           bool            `json:"healthy"`
	BlockIngestion    *BlockIngestion `json:"blockIngestion"`
	ChainBootstrapped bool            `json:"chainBootstrapped"`
}

type health struct {
	lock            sync.RWMutex
	newBestBlock    time.Time
	bestBlockID     thor.Bytes32
	bootstrapStatus bool
	blockInterval   time.Duration
	repo            *chain.Repository
	node            node.Network
}

func newHealth(repo *chain.Repository, node node.Network, blockInterval time.Duration) *health {
	h := &health{
		repo:          repo,
		node:          node,
		blockInterval: blockInterval,
	}
	go h.run()
	return h
}

const delayBuffer = 5 * time.Second

func (h *health) run() {
	ticker := time.NewTicker(time.Second)
	syncedChan := h.node.Synced()

	go func() {
		<-syncedChan
		h.BootstrapStatus(true)
	}()

	for {
		<-ticker.C
		bestID := h.repo.BestBlockSummary().Header.ID()
		if bestID != h.bestBlockID {
			h.NewBestBlock(bestID)
		}
	}
}

func (h *health) status() (*Status, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	blockIngest := &BlockIngestion{
		ID:        h.bestBlockID,
		Timestamp: &h.newBestBlock,
	}

	healthy := time.Since(h.newBestBlock) <= h.blockInterval+delayBuffer && // less than 10 secs have passed since a new block was received
		h.bootstrapStatus

	return &Status{
		Healthy:           healthy,
		BlockIngestion:    blockIngest,
		ChainBootstrapped: h.bootstrapStatus,
	}, nil
}

func (h *health) NewBestBlock(ID thor.Bytes32) {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.newBestBlock = time.Now()
	h.bestBlockID = ID
}

func (h *health) BootstrapStatus(bootstrapStatus bool) {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.bootstrapStatus = bootstrapStatus
}

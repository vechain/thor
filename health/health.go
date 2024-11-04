// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
	"fmt"
	"sync"
	"time"

	"github.com/vechain/thor/v2/thor"
)

type BlockIngestion struct {
	BestBlock                   *thor.Bytes32 `json:"bestBlock"`
	BestBlockIngestionTimestamp *time.Time    `json:"bestBlockIngestionTimestamp"`
}

type Status struct {
	Healthy           bool            `json:"healthy"`
	BlockIngestion    *BlockIngestion `json:"blockIngestion"`
	ChainBootstrapped bool            `json:"chainBootstrapped"`
}

type Health struct {
	lock              sync.RWMutex
	newBestBlock      time.Time
	bestBlockID       *thor.Bytes32
	bootstrapStatus   bool
	timeBetweenBlocks time.Duration
}

const delayBuffer = 5 * time.Second

func NewSolo(timeBetweenBlocks time.Duration) *Health {
	return &Health{
		timeBetweenBlocks: timeBetweenBlocks + delayBuffer,
		// there is no bootstrap in solo mode
		bootstrapStatus: true,
	}
}
func New(timeBetweenBlocks time.Duration) *Health {
	return &Health{
		timeBetweenBlocks: timeBetweenBlocks + delayBuffer,
	}
}

func (h *Health) Status() (*Status, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	blockIngest := &BlockIngestion{
		BestBlock:                   h.bestBlockID,
		BestBlockIngestionTimestamp: &h.newBestBlock,
	}

	healthy := time.Since(h.newBestBlock) <= h.timeBetweenBlocks && // less than 10 secs have passed since a new block was received
		h.bootstrapStatus

	fmt.Println("time between blocks", time.Since(h.newBestBlock).Seconds(), "of max", h.timeBetweenBlocks.Seconds())

	return &Status{
		Healthy:           healthy,
		BlockIngestion:    blockIngest,
		ChainBootstrapped: h.bootstrapStatus,
	}, nil
}

func (h *Health) NewBestBlock(ID thor.Bytes32) {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.newBestBlock = time.Now()
	h.bestBlockID = &ID
}

func (h *Health) BootstrapStatus(bootstrapStatus bool) {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.bootstrapStatus = bootstrapStatus
}

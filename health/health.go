// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package health

import (
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
	lock            sync.RWMutex
	newBestBlock    time.Time
	bestBlockID     *thor.Bytes32
	bootstrapStatus bool
}

func (h *Health) NewBestBlock(ID thor.Bytes32) {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.newBestBlock = time.Now()
	h.bestBlockID = &ID
}

func (h *Health) Status() (*Status, error) {
	h.lock.RLock()
	defer h.lock.RUnlock()

	blockIngest := &BlockIngestion{
		BestBlock:                   h.bestBlockID,
		BestBlockIngestionTimestamp: &h.newBestBlock,
	}

	healthy := time.Since(h.newBestBlock) <= 10*time.Second && // less than 10 secs have passed since a new block was received
		h.bootstrapStatus

	return &Status{
		Healthy:           healthy,
		BlockIngestion:    blockIngest,
		ChainBootstrapped: h.bootstrapStatus,
	}, nil
}

func (h *Health) BootstrapStatus(bootstrapStatus bool) {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.bootstrapStatus = bootstrapStatus
}

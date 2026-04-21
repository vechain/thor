// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// ContractStats holds CHAINID opcode call counts for a single contract address.
type ContractStats struct {
	Total      uint64 `json:"total"`
	Regular    uint64 `json:"regular"`
	Delegate   uint64 `json:"delegate"`
	FirstBlock uint64 `json:"first_block"`
	LastBlock  uint64 `json:"last_block"`
}

// ChainIDTracker tracks per-contract CHAINID opcode usage in memory and flushes to disk periodically.
type ChainIDTracker struct {
	mu      sync.Mutex
	stats   map[common.Address]*ContractStats
	outPath string
}

var globalChainIDTracker *ChainIDTracker

// InitChainIDTracker creates the global tracker, loading any existing data from outPath.
func InitChainIDTracker(outPath string) error {
	t := &ChainIDTracker{
		stats:   make(map[common.Address]*ContractStats),
		outPath: outPath,
	}
	if data, err := os.ReadFile(outPath); err == nil {
		var m map[string]*ContractStats
		if json.Unmarshal(data, &m) == nil {
			for hexAddr, s := range m {
				var addr common.Address
				addr.SetBytes(common.FromHex(hexAddr))
				t.stats[addr] = s
			}
		}
	}
	globalChainIDTracker = t
	return nil
}

// GetChainIDTracker returns the global tracker (may be nil if not initialised).
func GetChainIDTracker() *ChainIDTracker {
	return globalChainIDTracker
}

// Record increments counters for codeAddr (the implementation) and, for delegate calls where the
// proxy address differs, also for selfAddr (the proxy/storage context).
func (t *ChainIDTracker) Record(codeAddr *common.Address, selfAddr common.Address, isDelegateCall bool, blockNum uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if codeAddr != nil {
		t.recordLocked(*codeAddr, isDelegateCall, blockNum)
	}
	if isDelegateCall && codeAddr != nil && selfAddr != *codeAddr {
		t.recordLocked(selfAddr, isDelegateCall, blockNum)
	}
}

func (t *ChainIDTracker) recordLocked(addr common.Address, isDelegateCall bool, blockNum uint64) {
	s, ok := t.stats[addr]
	if !ok {
		s = &ContractStats{FirstBlock: blockNum}
		t.stats[addr] = s
	}
	s.Total++
	if isDelegateCall {
		s.Delegate++
	} else {
		s.Regular++
	}
	s.LastBlock = blockNum
}

// Flush writes the current stats to disk atomically.
func (t *ChainIDTracker) Flush() {
	t.mu.Lock()
	m := make(map[string]*ContractStats, len(t.stats))
	for addr, s := range t.stats {
		m[addr.Hex()] = s
	}
	t.mu.Unlock()

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	tmp := t.outPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, t.outPath)
}

// Start launches a goroutine that flushes every 30 seconds and performs a final flush when ctx is cancelled.
func (t *ChainIDTracker) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				t.Flush()
			case <-ctx.Done():
				t.Flush()
				return
			}
		}
	}()
}

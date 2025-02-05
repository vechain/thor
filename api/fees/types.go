// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
)

type Fees struct {
	data *FeesData
	done chan struct{}
}
type FeeCacheEntry struct {
	baseFee      *hexutil.Big
	gasUsedRatio float64
}
type FeesData struct {
	repo              *chain.Repository
	cache             *cache.PrioCache
	bft               bft.Committer
	cacheSize         uint32 // The max size of the cache when full.
	maxBacktraceLimit uint32 // The max number of blocks to backtrace.
}
type GetFeesHistory struct {
	OldestBlock   *uint32        `json:"oldestBlock"`
	BaseFees      []*hexutil.Big `json:"baseFees"`
	GasUsedRatios []float64      `json:"gasUsedRatios"`
}

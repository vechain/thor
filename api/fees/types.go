// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
)

const maxBlockFetchers = 8 // Maximum number of concurrent block fetchers.

type Fees struct {
	repo  *chain.Repository
	bft   bft.Committer
	cache *FeesCache
	done  chan struct{}
}
type FeeCacheEntry struct {
	baseFee      *hexutil.Big
	gasUsedRatio float64
}
type FeesCache struct {
	repo           *chain.Repository
	cache          *cache.PrioCache
	size           int    // The max size of the cache when full.
	backtraceLimit uint32 // The max number of blocks to backtrace.
	fixedSize      uint32 // The max size of the cache (fixed in the code).
}
type GetFeesHistory struct {
	OldestBlock   *uint32        `json:"oldestBlock"`
	BaseFees      []*hexutil.Big `json:"baseFees"`
	GasUsedRatios []float64      `json:"gasUsedRatios"`
}

type blockData struct {
	blockRevision *utils.Revision
	blockSummary  *chain.BlockSummary
	err           error
}

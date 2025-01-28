// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package fees

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/chain"
)

const (
	// maxBlockFetchers is the max number of goroutines to spin up to pull blocks
	// for the fee history calculation.
	maxBlockFetchers = 8
	// maxNumberOfBlocks is the max number of blocks allowed to be returned.
	maxNumberOfBlocks = 1024
)

type blockData struct {
	blockRevision *utils.Revision
	blockSummary  *chain.BlockSummary
	err           error
}

type GetFeesHistory struct {
	OldestBlock   *uint32        `json:"oldestBlock"`
	BaseFees      []*hexutil.Big `json:"baseFees"`
	GasUsedRatios []float64      `json:"gasUsedRatios"`
}

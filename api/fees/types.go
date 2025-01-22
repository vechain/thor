package fees

import (
	"math/big"

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
	OldestBlock   *uint32    `json:"oldestBlock"`
	BaseFees      []*big.Int `json:"baseFees"`
	GasUsedRatios []float64  `json:"gasUsedRatios"`
}

package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
)

type chainReader interface {
	IsNotFound(error) bool
	GetBlockHeader(cry.Hash) (*block.Header, error)
}

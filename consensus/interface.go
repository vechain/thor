package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
)

type ChainReader interface {
	IsNotFound(err error) bool
	GetBlockHeader(hash cry.Hash) (*block.Header, error)
}

type Stater interface {
}

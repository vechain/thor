package network

import (
	"github.com/vechain/thor/block"
)

type service interface {
	UpdateBlockPool(block.Block)
	BePacked(block.Block)
	GetIP() string
}

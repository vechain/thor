package network

import "github.com/vechain/thor/block"

type Block struct {
	Body *block.Block
	TTL  int // time to live
}

type service interface {
	UpdateBlockPool(Block)
	BePacked(Block)
	GetIP() string
}

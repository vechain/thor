package node

import (
	"container/list"

	"github.com/vechain/thor/block"
)

func InsertBlock(l *list.List, block block.Block) {
	l.PushBack(block)
}

func FrontBlock(l *list.List) block.Block {
	block, ok := l.Front().Value.(block.Block)
	if !ok {
		panic("front block")
	}
	return block
}

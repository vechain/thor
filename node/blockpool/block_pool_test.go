package blockpool

import (
	"testing"
	"time"

	"github.com/vechain/thor/block"
)

func Test_blockPool(t *testing.T) {
	bp := newBlockPool()

	bp.insertBlock(block.Block{})

	t.Log(bp.frontBlock())

	go func() {
		time.Sleep(3 * time.Second)
		bp.insertBlock(block.Block{})
	}()

	t.Log(bp.frontBlock())

	//t.Log(bp.frontBlock()) // will be block.
}

package node

import (
	"container/list"
	"errors"
	"log"
	"sync"

	"github.com/vechain/thor/block"
)

var errPoolClosed = errors.New("block pool closed")

type blockPool struct {
	l      *list.List
	mutex  *sync.Mutex
	wg     *sync.WaitGroup
	closed bool
}

func newBlockPool() *blockPool {
	wg := new(sync.WaitGroup)
	wg.Add(1)

	return &blockPool{
		l:      list.New(),
		mutex:  new(sync.Mutex),
		wg:     wg,
		closed: false}
}

func (bp *blockPool) insertBlock(block block.Block) {
	defer bp.wg.Done()

	bp.mutex.Lock()
	defer bp.mutex.Unlock()

	bp.l.PushBack(block)
}

func (bp *blockPool) frontBlock() (block.Block, error) {
	bp.wg.Wait()
	defer bp.wg.Add(1)

	bp.mutex.Lock()
	defer bp.mutex.Unlock()

	if bp.closed {
		return block.Block{}, errPoolClosed
	}

	block, ok := bp.l.Remove(bp.l.Front()).(block.Block)
	if !ok {
		log.Fatalln(errors.New("front block"))
	}

	return block, nil
}

func (bp *blockPool) close() {
	bp.wg.Done()

	bp.mutex.Lock()
	defer bp.mutex.Unlock()

	bp.closed = true
}

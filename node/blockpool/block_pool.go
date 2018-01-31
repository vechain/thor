package blockpool

import (
	"container/list"
	"errors"
	"log"
	"sync"

	"github.com/vechain/thor/block"
)

var errPoolClosed = errors.New("block pool closed")

type BlockPool struct {
	l      *list.List
	mutex  *sync.Mutex
	wg     *sync.WaitGroup
	closed bool
}

func New() *BlockPool {
	wg := new(sync.WaitGroup)
	wg.Add(1)

	return &BlockPool{
		l:      list.New(),
		mutex:  new(sync.Mutex),
		wg:     wg,
		closed: false}
}

func (bp *BlockPool) InsertBlock(block block.Block) {
	defer bp.wg.Done()

	bp.mutex.Lock()
	defer bp.mutex.Unlock()

	bp.l.PushBack(block)
}

func (bp *BlockPool) FrontBlock() (block.Block, error) {
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

func (bp *BlockPool) Close() {
	bp.wg.Done()

	bp.mutex.Lock()
	defer bp.mutex.Unlock()

	bp.closed = true
}

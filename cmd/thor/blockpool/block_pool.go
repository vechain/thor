package blockpool

import (
	"container/list"
	"errors"
	"log"
	"sync"

	"github.com/vechain/thor/cmd/thor/network"
)

var errPoolClosed = errors.New("block pool closed")

type BlockPool struct {
	l      *list.List
	mutex  sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

func New() *BlockPool {
	bp := &BlockPool{
		l:      list.New(),
		closed: false}
	bp.wg.Add(1)
	return bp
}

func (bp *BlockPool) InsertBlock(block network.Block) {
	defer bp.wg.Done()

	bp.mutex.Lock()
	defer bp.mutex.Unlock()

	bp.l.PushBack(block)
}

func (bp *BlockPool) FrontBlock() (network.Block, error) {
	bp.wg.Wait()
	defer bp.wg.Add(1)

	bp.mutex.Lock()
	defer bp.mutex.Unlock()

	if bp.closed {
		return network.Block{}, errPoolClosed
	}

	block, ok := bp.l.Remove(bp.l.Front()).(network.Block)
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

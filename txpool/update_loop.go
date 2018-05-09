package txpool

import (
	"time"

	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/thor"
)

func (pool *TxPool) updateLoop() {
	var (
		bestBlock = pool.chain.BestBlock()
		ticker    = time.NewTicker(time.Second)
	)

	for {
		select {
		case <-pool.done:
			return
		case <-ticker.C:
			currentBestBlock := pool.chain.BestBlock()
			if currentBestBlock.Header().ID() == bestBlock.Header().ID() {
				continue
			}
			pool.updateData(currentBestBlock)
			bestBlock = currentBestBlock
		}
	}
}

func (pool *TxPool) updateData(bestBlock *block.Block) {
	pool.rw.Lock()
	defer pool.rw.Unlock()

	log := log15.New("txpool", pool)
	allObjs := pool.parseTxObjects()
	pool.data.pending = make(txObjects, 0, len(allObjs))
	pool.data.dirty = false
	pool.data.sorted = false

	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		log.Error("err", err)
		return
	}

	baseGasPrice := builtin.Params.Native(st).Get(thor.KeyBaseGasPrice)
	traverser := pool.chain.NewTraverser(bestBlock.Header().ID())
	bestBlockNum := bestBlock.Header().Number()

	//can be pendinged txObjects
	for id, obj := range allObjs {
		if obj.tx.IsExpired(bestBlockNum) || time.Now().Unix()-obj.creationTime > int64(pool.config.Lifetime) {
			pool.Remove(id)
			continue
		}

		if obj.status == Queued {
			state := obj.currentState(pool.chain, bestBlockNum)
			if state != Pending {
				continue
			}

			obj.status = state
			obj.overallGP = obj.tx.OverallGasPrice(baseGasPrice, bestBlockNum, func(num uint32) thor.Bytes32 {
				return traverser.Get(num).ID()
			})
			pool.data.all.Set(id, obj)
		}

		if obj.status == Pending {
			pool.data.pending = append(pool.data.pending, obj)
		}
	}
}

func (pool *TxPool) parseTxObjects() map[thor.Bytes32]*txObject {
	allObjs := make(map[thor.Bytes32]*txObject)
	pool.data.all.ForEach(func(entry *cache.Entry) bool {
		if obj, ok := entry.Value.(*txObject); ok {
			if key, ok := entry.Key.(thor.Bytes32); ok {
				allObjs[key] = obj
				return true
			}
		}
		return false
	})

	return allObjs
}

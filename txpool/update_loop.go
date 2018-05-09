package txpool

import (
	"time"

	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
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
	log := log15.New("txpool", pool)
	allObjs := pool.entry.dumpAll()
	pending := make(txObjects, 0, len(allObjs))

	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		log.Error("err", err)
		return
	}

	baseGasPrice := builtin.Params.Native(st).Get(thor.KeyBaseGasPrice)
	traverser := pool.chain.NewTraverser(bestBlock.Header().ID())
	bestBlockNum := bestBlock.Header().Number()

	//can be pendinged txObjects
	for _, obj := range allObjs {
		if obj.tx.IsExpired(bestBlockNum) || time.Now().Unix()-obj.creationTime > int64(pool.config.Lifetime) {
			pool.entry.delete(obj.tx.ID())
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
			pool.entry.save(obj)
		}

		if obj.status == Pending {
			pending = append(pending, obj)
		}
	}

	pool.entry.cachePending(pending)
}

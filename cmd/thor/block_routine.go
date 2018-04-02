package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/vechain/thor/block"
	Chain "github.com/vechain/thor/chain"
	"github.com/vechain/thor/cmd/thor/minheap"
	"github.com/vechain/thor/comm"
	Consensus "github.com/vechain/thor/consensus"
	Logdb "github.com/vechain/thor/logdb"
	Packer "github.com/vechain/thor/packer"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	Txpool "github.com/vechain/thor/txpool"
)

type blockRoutineContext struct {
	ctx              context.Context
	communicator     *comm.Communicator
	chain            *Chain.Chain
	packedChan       chan *packedEvent
	bestBlockUpdated chan struct{}
}

type newBlockEvent struct {
	Blk      *block.Block
	Receipts tx.Receipts
	Trunk    bool
}

type packedEvent struct {
	blk      *block.Block
	receipts tx.Receipts
	ack      chan struct{}
}

type orphan struct {
	blk       *block.Block
	timestamp uint64 // 块成为 orpahn 的时间, 最多维持 5 分钟
}

func consentLoop(context *blockRoutineContext, consensus *Consensus.Consensus, logdb *Logdb.LogDB) {
	futures := minheap.NewBlockMinHeap()
	orphanMap := make(map[thor.Hash]*orphan)
	consent := func(blk *block.Block) error {
		trunk, receipts, err := consensus.Consent(blk, uint64(time.Now().Unix()))
		if err != nil {
			log.Warn(fmt.Sprintf("received new block(#%v bad)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size(), "err", err.Error())
			if Consensus.IsFutureBlock(err) {
				futures.Push(blk)
			} else if Consensus.IsParentNotFound(err) {
				parentID := blk.Header().ParentID()
				if _, ok := orphanMap[parentID]; !ok {
					orphanMap[parentID] = &orphan{blk: blk, timestamp: uint64(time.Now().Unix())}
				}
			}
			return err
		}

		updateChain(context.chain, context.communicator, &newBlockEvent{
			Blk:      blk,
			Trunk:    trunk,
			Receipts: receipts,
		}, logdb, context.bestBlockUpdated)

		return nil
	}

	subChan := make(chan *block.Block, 100)
	sub := context.communicator.SubscribeBlock(subChan)
	ticker := time.NewTicker(time.Duration(thor.BlockInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-context.ctx.Done():
			sub.Unsubscribe()
			return
		case <-ticker.C:
			if blk := futures.Pop(); blk != nil {
				consent(blk)
			}
		case blk := <-subChan:
			if err := consent(blk); err != nil {
				break
			}

			if orphan, ok := orphanMap[blk.Header().ID()]; ok {
				if orphan.timestamp+300 >= uint64(time.Now().Unix()) {
					err := consent(orphan.blk)
					if err != nil && Consensus.IsParentNotFound(err) {
						continue
					}
				}
				delete(orphanMap, blk.Header().ID())
			}
		case packed := <-context.packedChan:
			if trunk, err := consensus.IsTrunk(packed.blk.Header()); err == nil {
				updateChain(context.chain, context.communicator, &newBlockEvent{
					Blk:      packed.blk,
					Trunk:    trunk,
					Receipts: packed.receipts,
				}, logdb, context.bestBlockUpdated)
				packed.ack <- struct{}{}
			}
		}
	}
}

func packLoop(context *blockRoutineContext, packer *Packer.Packer, txpool *Txpool.TxPool, privateKey *ecdsa.PrivateKey) {
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()

	for !context.communicator.IsSynced() {
		select {
		case <-context.ctx.Done():
			return
		default:
			log.Warn("has not synced")
			time.Sleep(1 * time.Second)
		}
	}

	var (
		ts        uint64
		adopt     Packer.Adopt
		commit    Packer.Commit
		bestBlock *block.Block
		err       error
	)

	for {
		timer.Reset(2 * time.Second)

		select {
		case <-context.ctx.Done():
			return
		case <-context.bestBlockUpdated:
			bestBlock, err = context.chain.GetBestBlock()
			if err != nil {
				log.Error(fmt.Sprintf("%v", err))
				break
			}

			ts, adopt, commit, err = packer.Prepare(bestBlock.Header(), uint64(time.Now().Unix()))
			if err != nil {
				log.Error(fmt.Sprintf("%v", err))
				break
			}
		case <-timer.C:
			now := uint64(time.Now().Unix())
			if now >= ts && now < ts+thor.BlockInterval {
				ts = 0
				pendings := txpool.Pending()

				for _, tx := range pendings {
					err := adopt(tx)
					if Packer.IsGasLimitReached(err) {
						break
					}
					txpool.OnProcessed(tx.ID(), err)
				}

				blk, receipts, err := commit(privateKey)
				if err != nil {
					break
				}

				log.Info(fmt.Sprintf("proposed new block(#%v)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size())
				pe := &packedEvent{
					blk:      blk,
					receipts: receipts,
					ack:      make(chan struct{}),
				}
				context.packedChan <- pe
				<-pe.ack
			}
		}
	}
}

func updateChain(
	chain *Chain.Chain,
	communicator *comm.Communicator,
	newBlk *newBlockEvent,
	logdb *Logdb.LogDB,
	bestBlockUpdated chan struct{},
) {
	fork, err := chain.AddBlock(newBlk.Blk, newBlk.Receipts, newBlk.Trunk)
	if err != nil {
		log.Error(fmt.Sprintf("%v", err))
		return
	}

	log.Info(
		fmt.Sprintf("add new block to chain(#%v %v)", newBlk.Blk.Header().Number(), newBlk.Trunk),
		"id", newBlk.Blk.Header().ID(),
		"size", newBlk.Blk.Size(),
	)

	if newBlk.Trunk {
		select {
		case bestBlockUpdated <- struct{}{}:
		default:
		}
		communicator.BroadcastBlock(newBlk.Blk)

		// 日志
		logs := []*Logdb.Log{}
		var index uint32
		txs := newBlk.Blk.Transactions()
		for i, receipt := range newBlk.Receipts {
			for _, output := range receipt.Outputs {
				tx := txs[i]
				signer, err := tx.Signer()
				if err != nil {
					log.Error(fmt.Sprintf("%v", err))
					return
				}
				for _, log := range output.Logs {
					logs = append(logs, Logdb.NewLog(newBlk.Blk.Header(), index, tx.ID(), signer, log))
				}
				index++
			}
		}
		forkIDs := make([]thor.Hash, len(fork.Branch), len(fork.Branch))
		for i, blk := range fork.Branch {
			forkIDs[i] = blk.Header().ID()
		}
		logdb.Insert(logs, forkIDs)
	}
}

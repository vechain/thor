package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/vechain/thor/co"

	"github.com/ethereum/go-ethereum/common/mclock"
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
	txpool           *Txpool.TxPool
	packedChan       chan *packedEvent
	bestBlockUpdated chan *block.Block // must be >=1 buffer chan
}

func produceBlock(
	context *blockRoutineContext,
	consensus *Consensus.Consensus,
	packer *Packer.Packer,
	privateKey *ecdsa.PrivateKey,
	logdb *Logdb.LogDB,
) {
	var goes co.Goes

	goes.Go(func() {
		consentLoop(context, consensus, logdb)
	})
	goes.Go(func() {
		packLoop(context, packer, privateKey)
	})

	log.Info("Block consensus started")
	log.Info("Block packer started")
	goes.Wait()
	log.Info("Block consensus stoped")
	log.Info("Block packer stoped")
}

type orphan struct {
	blk       *block.Block
	timestamp uint64 // 块成为 orpahn 的时间, 最多维持 5 分钟
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

func consentLoop(context *blockRoutineContext, consensus *Consensus.Consensus, logdb *Logdb.LogDB) {
	futures := minheap.NewBlockMinHeap()
	orphanMap := make(map[thor.Bytes32]*orphan)
	updateChainFn := func(newBlk *newBlockEvent) {
		updateChain(context, logdb, newBlk)
	}
	consentFn := func(blk *block.Block) error {
		trunk, receipts, err := consensus.Consent(blk, uint64(time.Now().Unix()))
		if err != nil {
			//log.Warn(fmt.Sprintf("received new block(#%v bad)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size(), "err", err.Error())
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

		updateChainFn(&newBlockEvent{
			Blk:      blk,
			Trunk:    trunk,
			Receipts: receipts})

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
				consentFn(blk)
			}
		case blk := <-subChan:
			if err := consentFn(blk); err != nil {
				break
			}

			if orphan, ok := orphanMap[blk.Header().ID()]; ok {
				if orphan.timestamp+300 >= uint64(time.Now().Unix()) {
					err := consentFn(orphan.blk)
					if err != nil && Consensus.IsParentNotFound(err) {
						continue
					}
				}
				delete(orphanMap, blk.Header().ID())
			}
		case packed := <-context.packedChan:
			if trunk, err := consensus.IsTrunk(packed.blk.Header()); err == nil {
				updateChainFn(&newBlockEvent{
					Blk:      packed.blk,
					Trunk:    trunk,
					Receipts: packed.receipts})
				packed.ack <- struct{}{}
			}
		}
	}
}

func packLoop(context *blockRoutineContext, packer *Packer.Packer, privateKey *ecdsa.PrivateKey) {
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()

	for !context.communicator.IsSynced() {
		select {
		case <-context.ctx.Done():
			return
		default:
			time.Sleep(1 * time.Second)
		}
	}
	log.Info("Chain data has synced")

	var (
		ts        uint64
		adopt     Packer.Adopt
		commit    Packer.Commit
		bestBlock *block.Block
		err       error
	)

	bestBlock, err = context.chain.GetBestBlock()
	if err != nil {
		log.Error(fmt.Sprintf("%v", err))
		return
	}

	sendBestBlock(context.bestBlockUpdated, bestBlock)

	for {
		timer.Reset(2 * time.Second)

		select {
		case <-context.ctx.Done():
			return
		case bestBlock = <-context.bestBlockUpdated:
			ts, adopt, commit, err = packer.Prepare(bestBlock.Header(), uint64(time.Now().Unix()))
			if err != nil {
				log.Error(fmt.Sprintf("%v", err))
				break
			}
		case <-timer.C:
			now := uint64(time.Now().Unix())
			if now >= ts && now < ts+thor.BlockInterval {
				ts = 0
				pack(context.txpool, packer, adopt, commit, privateKey, context.packedChan)
			} else if ts > now {
				//fmt.Printf("after %v seconds to pack.\r\n", ts-now)
			}
		}
	}
}

func pack(
	txpool *Txpool.TxPool,
	packer *Packer.Packer,
	adopt Packer.Adopt,
	commit Packer.Commit,
	privateKey *ecdsa.PrivateKey,
	packedChan chan *packedEvent,
) {
	adoptTx := func() {
		for _, tx := range txpool.Pending() {
			err := adopt(tx)
			switch {
			case Packer.IsBadTx(err) || Packer.IsKnownTx(err):
				txpool.Remove(tx.ID())
			case Packer.IsGasLimitReached(err):
				return
			default:
			}
		}
	}

	startTime := mclock.Now()
	adoptTx()
	blk, receipts, err := commit(privateKey)
	if err != nil {
		log.Error(fmt.Sprintf("%v", err))
		return
	}
	elapsed := mclock.Now() - startTime

	if elapsed > 0 {
		gasUsed := blk.Header().GasUsed()
		// calc target gas limit only if gas used above third of gas limit
		if gasUsed > blk.Header().GasLimit()/3 {
			targetGasLimit := uint64(thor.TolerableBlockPackingTime) * gasUsed / uint64(elapsed)
			packer.SetTargetGasLimit(targetGasLimit)
		}
	}

	//log.Info(fmt.Sprintf("proposed new block(#%v)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size())
	pe := &packedEvent{
		blk:      blk,
		receipts: receipts,
		ack:      make(chan struct{}),
	}
	packedChan <- pe
	<-pe.ack
}

func updateChain(
	context *blockRoutineContext,
	logdb *Logdb.LogDB,
	newBlk *newBlockEvent,
) {
	fork, err := context.chain.AddBlock(newBlk.Blk, newBlk.Receipts, newBlk.Trunk)
	if err != nil {
		log.Error(fmt.Sprintf("%v", err))
		return
	}

	// log.Info(
	// 	fmt.Sprintf("add new block to chain(#%v %v)", newBlk.Blk.Header().Number(), newBlk.Trunk),
	// 	"id", newBlk.Blk.Header().ID(),
	// 	"size", newBlk.Blk.Size(),
	// )

	if newBlk.Trunk {
		sendBestBlock(context.bestBlockUpdated, newBlk.Blk)
		context.communicator.BroadcastBlock(newBlk.Blk)

		// fork
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
		forkIDs := make([]thor.Bytes32, len(fork.Branch), len(fork.Branch))
		for i, blk := range fork.Branch {
			forkIDs[i] = blk.Header().ID()
			for _, tx := range blk.Transactions() {
				context.txpool.Add(tx)
			}
		}
		logdb.Insert(logs, forkIDs)
	}
}

func sendBestBlock(bestBlockUpdated chan *block.Block, blk *block.Block) {
	for {
		select {
		case bestBlockUpdated <- blk:
			return
		case <-bestBlockUpdated:
		}
	}
}

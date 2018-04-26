package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cmd/thor/minheap"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/comm"
	Consensus "github.com/vechain/thor/consensus"
	"github.com/vechain/thor/eventdb"
	Packer "github.com/vechain/thor/packer"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/transferdb"
	"github.com/vechain/thor/tx"
)

func produceBlock(ctx context.Context, components *components, eventDB *eventdb.EventDB, transferDB *transferdb.TransferDB) {
	var goes co.Goes

	packedChan := make(chan *packedEvent)
	bestBlockUpdated := make(chan *block.Block, 1)

	goes.Go(func() { consentLoop(ctx, components, eventDB, transferDB, bestBlockUpdated, packedChan) })
	goes.Go(func() { packLoop(ctx, components, bestBlockUpdated, packedChan) })

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
	Blk       *block.Block
	Receipts  tx.Receipts
	Transfers [][]tx.Transfers
	Trunk     bool
	IsSynced  bool
}

type packedEvent struct {
	blk       *block.Block
	receipts  tx.Receipts
	transfers [][]tx.Transfers
	ack       chan struct{}
}

func consentLoop(
	ctx context.Context,
	components *components,
	eventDB *eventdb.EventDB,
	transferDB *transferdb.TransferDB,
	bestBlockUpdated chan *block.Block,
	packedChan chan *packedEvent,
) {
	futures := minheap.NewBlockMinHeap()
	orphanMap := make(map[thor.Bytes32]*orphan)
	updateChainFn := func(newBlk *newBlockEvent) error {
		return updateChain(ctx, components, eventDB, transferDB, newBlk, bestBlockUpdated)
	}
	consentFn := func(blk *block.Block, isSynced bool) error {
		trunk, receipts, transfers, err := components.consensus.Consent(blk, uint64(time.Now().Unix()))
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

		return updateChainFn(&newBlockEvent{
			Blk:       blk,
			Trunk:     trunk,
			Receipts:  receipts,
			Transfers: transfers,
			IsSynced:  isSynced,
		})
	}

	subChan := make(chan *comm.NewBlockEvent, 100)
	sub := components.communicator.SubscribeBlock(subChan)

	ticker := time.NewTicker(time.Duration(thor.BlockInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			sub.Unsubscribe()
			return
		case <-ticker.C:
			if blk := futures.Pop(); blk != nil {
				consentFn(blk, false)
			}
		case ev := <-subChan:
			if err := consentFn(ev.Block, ev.IsSynced); err != nil {
				break
			}

			if orphan, ok := orphanMap[ev.Block.Header().ID()]; ok {
				if orphan.timestamp+300 >= uint64(time.Now().Unix()) {
					if err := consentFn(orphan.blk, false); err != nil && Consensus.IsParentNotFound(err) {
						break
					}
				}
				delete(orphanMap, ev.Block.Header().ID())
			}
		case packed := <-packedChan:
			if trunk, err := components.consensus.IsTrunk(packed.blk.Header()); err == nil {
				updateChainFn(&newBlockEvent{
					Blk:       packed.blk,
					Receipts:  packed.receipts,
					Transfers: packed.transfers,
					Trunk:     trunk,
					IsSynced:  false,
				})
				packed.ack <- struct{}{}
			}
		}
	}
}

func packLoop(
	ctx context.Context,
	components *components,
	bestBlockUpdated chan *block.Block,
	packedChan chan *packedEvent,
) {
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()

	if !components.communicator.IsSynced() {
		log.Warn("Chain data has not synced, waiting...")
	}
	for !components.communicator.IsSynced() {
		select {
		case <-ctx.Done():
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

	bestBlock, err = components.chain.GetBestBlock()
	if err != nil {
		log.Error(fmt.Sprintf("%v", err))
		return
	}
	sendBestBlock(bestBlockUpdated, bestBlock)

	for {
		timer.Reset(2 * time.Second)

		select {
		case <-ctx.Done():
			return
		case bestBlock = <-bestBlockUpdated:
			ts, adopt, commit, err = components.packer.Prepare(bestBlock.Header(), uint64(time.Now().Unix()))
			if err != nil {
				log.Error(fmt.Sprintf("%v", err))
				break
			}
		case <-timer.C:
			now := uint64(time.Now().Unix())
			if now >= ts && now < ts+thor.BlockInterval {
				ts = 0
				pack(components, adopt, commit, packedChan)
			} else if ts > now {
				//fmt.Printf("after %v seconds to pack.\r\n", ts-now)
			}
		}
	}
}

func pack(components *components, adopt Packer.Adopt, commit Packer.Commit, packedChan chan *packedEvent) {
	adoptTx := func() {
		for _, tx := range components.txpool.Pending() {
			err := adopt(tx)
			switch {
			case Packer.IsBadTx(err) || Packer.IsKnownTx(err):
				components.txpool.Remove(tx.ID())
			case Packer.IsGasLimitReached(err):
				return
			default:
			}
		}
	}

	startTime := mclock.Now()
	adoptTx()
	blk, receipts, transfers, err := commit(components.packer.privateKey)
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
			components.packer.SetTargetGasLimit(targetGasLimit)
		}
	}

	pe := &packedEvent{
		blk:       blk,
		receipts:  receipts,
		transfers: transfers,
		ack:       make(chan struct{}),
	}
	packedChan <- pe
	<-pe.ack
}

func updateChain(
	ctx context.Context,
	components *components,
	eventDB *eventdb.EventDB,
	transferDB *transferdb.TransferDB,
	newBlk *newBlockEvent,
	bestBlockUpdated chan *block.Block,
) error {
	fork, err := components.chain.AddBlock(newBlk.Blk, newBlk.Receipts, newBlk.Trunk)
	if err != nil {
		log.Error(fmt.Sprintf("%v", err))
		return err
	}

	if newBlk.Trunk {
		if !newBlk.IsSynced {
			header := newBlk.Blk.Header()
			if signer, err := header.Signer(); err == nil {
				log.Info("Best block updated",
					"number", header.Number(),
					"id", header.ID().AbbrevString(),
					"total-score", header.TotalScore(),
					"proposer", signer.String(),
				)
			}
		}

		sendBestBlock(bestBlockUpdated, newBlk.Blk)
		components.communicator.BroadcastBlock(newBlk.Blk)

		// fork
		var eventIndex uint32
		var transferIndex uint32
		txs := newBlk.Blk.Transactions()
		var events []*eventdb.Event
		var transfers []*transferdb.Transfer

		for i, receipt := range newBlk.Receipts {
			for j, output := range receipt.Outputs {
				tx := txs[i]
				signer, err := tx.Signer()
				if err != nil {
					log.Error(fmt.Sprintf("%v", err))
					return err
				}
				header := newBlk.Blk.Header()
				for _, e := range output.Events {
					events = append(events, eventdb.NewEvent(header, eventIndex, tx.ID(), signer, e))
					eventIndex++
				}
				for _, transfer := range newBlk.Transfers[i][j] {
					transfers = append(transfers, transferdb.NewTransfer(header, transferIndex, tx.ID(), signer, transfer))
					transferIndex++
				}
			}
		}

		forkIDs := make([]thor.Bytes32, len(fork.Branch), len(fork.Branch))
		for i, blk := range fork.Branch {
			forkIDs[i] = blk.Header().ID()
			for _, tx := range blk.Transactions() {
				components.txpool.Add(tx)
			}
		}

		eventDB.Insert(events, forkIDs)
		transferDB.Insert(transfers, forkIDs)
	}

	return nil
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

package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/vechain/thor/block"
	Chain "github.com/vechain/thor/chain"
	"github.com/vechain/thor/cmd/thor/minheap"
	"github.com/vechain/thor/comm"
	Consensus "github.com/vechain/thor/consensus"
	Packer "github.com/vechain/thor/packer"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	Txpool "github.com/vechain/thor/txpool"
)

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

func consentLoop(ctx context.Context, communicator *comm.Communicator, chain *Chain.Chain, packedChan chan *packedEvent, bestBlockUpdated chan struct{}, consensus *Consensus.Consensus) {
	futures := minheap.NewBlockMinHeap()
	orphanMap := make(map[thor.Hash]*orphan)
	consent := func(blk *block.Block) error {
		return consent(consensus, chain, communicator, futures, orphanMap, blk, bestBlockUpdated)
	}

	subChan := make(chan *block.Block, 100)
	sub := communicator.SubscribeBlock(subChan)
	ticker := time.NewTicker(time.Duration(thor.BlockInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
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

			now := uint64(time.Now().Unix())
			for id, orphan := range orphanMap {
				if orphan.timestamp+300 >= now {
					err := consent(orphan.blk)
					if err != nil && Consensus.IsParentNotFound(err) {
						continue
					}
				}
				delete(orphanMap, id)
			}
		case packed := <-packedChan:
			if trunk, err := consensus.IsTrunk(packed.blk.Header()); err == nil {
				updateChain(chain, communicator, &newBlockEvent{
					Blk:      packed.blk,
					Trunk:    trunk,
					Receipts: packed.receipts,
				}, bestBlockUpdated)
				packed.ack <- struct{}{}
			}
		}
	}
}

func consent(consensus *Consensus.Consensus, chain *Chain.Chain, communicator *comm.Communicator, futures *minheap.Blocks, orphanMap map[thor.Hash]*orphan, blk *block.Block, bestBlockUpdated chan struct{}) error {
	trunk, receipts, err := consensus.Consent(blk, uint64(time.Now().Unix()))
	if err != nil {
		log.Warn(fmt.Sprintf("received new block(#%v bad)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size(), "err", err.Error())
		if Consensus.IsFutureBlock(err) {
			futures.Push(blk)
		} else if Consensus.IsParentNotFound(err) {
			id := blk.Header().ID()
			if _, ok := orphanMap[id]; !ok {
				orphanMap[id] = &orphan{blk: blk, timestamp: uint64(time.Now().Unix())}
			}
		}
		return err
	}

	updateChain(chain, communicator, &newBlockEvent{
		Blk:      blk,
		Trunk:    trunk,
		Receipts: receipts,
	}, bestBlockUpdated)

	return nil
}

func packLoop(ctx context.Context, communicator *comm.Communicator, chain *Chain.Chain, packedChan chan *packedEvent, bestBlockUpdated chan struct{}, packer *Packer.Packer, txpool *Txpool.TxPool, privateKey *ecdsa.PrivateKey) {
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()

	for {
		timer.Reset(1 * time.Second)

		select {
		case <-ctx.Done():
			return
		case <-bestBlockUpdated:
			break
		case <-timer.C:
			if communicator.IsSynced() {
				bestBlock, err := chain.GetBestBlock()
				if err != nil {
					log.Error("%v", err)
					return
				}

				now := uint64(time.Now().Unix())
				if ts, adopt, commit, err := packer.Prepare(bestBlock.Header(), now); err == nil {
					waitSec := ts - now
					log.Info(fmt.Sprintf("waiting to propose new block(#%v)", bestBlock.Header().Number()+1), "after", fmt.Sprintf("%vs", waitSec))

					waitTime := time.NewTimer(time.Duration(waitSec) * time.Second)
					defer waitTime.Stop()

					select {
					case <-waitTime.C:
						pendings, err := txpool.Sorted(Txpool.Pending)
						if err != nil {
							break
						}
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
						packedChan <- pe
						<-pe.ack
					case <-bestBlockUpdated:
						break
					case <-ctx.Done():
						return
					}
				}
			} else {
				log.Warn("has not synced")
			}
		}
	}
}

func updateChain(chain *Chain.Chain, communicator *comm.Communicator, newBlk *newBlockEvent, bestBlockUpdated chan struct{}) {
	_, err := chain.AddBlock(newBlk.Blk, newBlk.Receipts, newBlk.Trunk)
	if err != nil {
		return
	}

	log.Info(
		fmt.Sprintf("received new block(#%v valid %v)", newBlk.Blk.Header().Number(), newBlk.Trunk),
		"id", newBlk.Blk.Header().ID(),
		"size", newBlk.Blk.Size(),
	)

	if newBlk.Trunk {
		select {
		case bestBlockUpdated <- struct{}{}:
		default:
		}
		communicator.BroadcastBlock(newBlk.Blk)
	}

	// 日志待写
}

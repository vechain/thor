package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type orphan struct {
	blk       *block.Block
	timestamp uint64 // 块成为 orpahn 的时间, 最多维持 5 分钟
}

type packedEvent struct {
	blk *block.Block
	ack chan struct{}
}

type consent struct {
	cm        *comm.Communicator
	ch        *chain.Chain
	cs        *consensus.Consensus
	futures   *futureBlocks
	orphanMap map[thor.Hash]*orphan
	feed      event.Feed
	feedScope event.SubscriptionScope
}

func newConsent(cm *comm.Communicator, ch *chain.Chain, stateCreator *state.Creator) *consent {
	return &consent{
		cm:        cm,
		ch:        ch,
		cs:        consensus.New(ch, stateCreator),
		futures:   newFutureBlocks(),
		orphanMap: make(map[thor.Hash]*orphan),
	}
}

func (c *consent) subscribeBestBlockUpdate(ch chan struct{}) event.Subscription {
	return c.feedScope.Track(c.feed.Subscribe(ch))
}

func (c *consent) consent(blk *block.Block) error {
	trunk, _, err := c.cs.Consent(blk, uint64(time.Now().Unix()))
	if err != nil {
		log.Warn(fmt.Sprintf("received new block(#%v bad)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size(), "err", err.Error())
		if consensus.IsFutureBlock(err) {
			c.futures.Push(blk)
		} else if consensus.IsParentNotFound(err) {
			id := blk.Header().ID()
			if _, ok := c.orphanMap[id]; !ok {
				c.orphanMap[id] = &orphan{blk: blk, timestamp: uint64(time.Now().Unix())}
			}
		}
		return err
	}

	c.ch.AddBlock(blk, trunk)

	if trunk == false {
		log.Info(fmt.Sprintf("received new block(#%v branch)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size())
		return nil
	}

	log.Info(fmt.Sprintf("received new block(#%v trunk)", blk.Header().Number()), "id", blk.Header().ID(), "size", blk.Size())
	c.cm.BroadcastBlock(blk)
	go func() {
		c.feed.Send(struct{}{})
	}()

	return nil
}

func (c *consent) run(ctx context.Context, packedChan chan packedEvent) {
	subChan := make(chan *block.Block, 100)
	sub := c.cm.SubscribeBlock(subChan)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			sub.Unsubscribe()
			return
		case <-ticker.C:
			if blk := c.futures.Pop(); blk != nil {
				c.consent(blk)
			}
		case blk := <-subChan:
			if err := c.consent(blk); err != nil {
				break
			}

			now := uint64(time.Now().Unix())
			for id, orphan := range c.orphanMap {
				if orphan.timestamp+300 >= now {
					err := c.consent(blk)
					if err != nil && consensus.IsParentNotFound(err) {
						continue
					}
				}
				delete(c.orphanMap, id)
			}
		case packed := <-packedChan:
			if trunk, err := c.cs.IsTrunk(packed.blk.Header()); err == nil {
				c.ch.AddBlock(packed.blk, trunk)
				if trunk {
					c.cm.BroadcastBlock(packed.blk)
				}
				packed.ack <- struct{}{}
			}
		}
	}
}

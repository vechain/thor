package comm

import (
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/vechain/thor/comm/proto"
	"github.com/vechain/thor/thor"
)

type announcement struct {
	newBlockID thor.Bytes32
	peer       *Peer
}

func (c *Communicator) announcementLoop() {
	const maxFetches = 3 // per block ID

	fetchingPeers := map[discover.NodeID]bool{}
	fetchingBlockIDs := map[thor.Bytes32]int{}

	fetchDone := make(chan *announcement)

	for {
		select {
		case <-c.ctx.Done():
			return
		case ann := <-fetchDone:
			delete(fetchingPeers, ann.peer.ID())
			if n := fetchingBlockIDs[ann.newBlockID] - 1; n > 0 {
				fetchingBlockIDs[ann.newBlockID] = n
			} else {
				delete(fetchingBlockIDs, ann.newBlockID)
			}
		case ann := <-c.announcementCh:
			if f, n := fetchingPeers[ann.peer.ID()], fetchingBlockIDs[ann.newBlockID]; !f && n < maxFetches {
				fetchingPeers[ann.peer.ID()] = true
				fetchingBlockIDs[ann.newBlockID] = n + 1

				c.goes.Go(func() {
					defer func() {
						select {
						case fetchDone <- ann:
						case <-c.ctx.Done():
						}
					}()
					c.fetchBlockByID(ann.peer, ann.newBlockID)
				})
			} else {
				ann.peer.logger.Debug("skip new block ID announcement")
			}
		}
	}
}

func (c *Communicator) fetchBlockByID(peer *Peer, newBlockID thor.Bytes32) {
	if _, err := c.chain.GetBlockHeader(newBlockID); err != nil {
		if !c.chain.IsNotFound(err) {
			peer.logger.Error("failed to get block header", "err", err)
		}
	} else {
		// already in chain
		return
	}
	result, err := proto.GetBlockByID{ID: newBlockID}.Call(c.ctx, peer)
	if err != nil {
		peer.logger.Debug("failed to get block by id", "err", err)
		return
	}
	if result.Block == nil {
		peer.logger.Debug("get nil block by id")
		return
	}
	c.newBlockFeed.Send(&NewBlockEvent{
		Block: result.Block,
	})
}

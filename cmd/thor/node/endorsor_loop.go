package node

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/thor"
)

// endorsorLoop is the loop dedicated to endorsing incoming block summaries
func (n *Node) endorsorLoop(ctx context.Context) {
	debugLog := func(str string, kv ...interface{}) {
		log.Info(str, append([]interface{}{"key", "edlp"}, kv...)...)
	}

	errLog := func(str string, kv ...interface{}) {
		log.Error(str, append([]interface{}{"key", "edlp"}, kv...)...)
	}

	debugLog("started endorser loop")
	defer debugLog("leaving endorser loop")

	// debugLog("waiting for synchronization...")
	// select {
	// case <-ctx.Done():
	// 	return
	// case <-n.comm.Synced():
	// }
	// debugLog("synchronization process done")

	var scope event.SubscriptionScope
	defer scope.Close()

	newBlockSummaryCh := make(chan *comm.NewBlockSummaryEvent)
	scope.Track(n.comm.SubscribeBlockSummary(newBlockSummaryCh))

	// cache that stores (leader_address, timestamp) kv pairs
	// In each round, the node only endorse one valid block summary from the same leader.
	endorsedLeader := cache.NewRandCache(int(thor.MaxBlockProposers))

	for {
		select {
		case <-ctx.Done():
			return

		case ev := <-newBlockSummaryCh:
			now := uint64(time.Now().Unix())
			best := n.repo.BestBlock()

			bs := ev.Summary

			leader, err := bs.Signer()
			if err != nil {
				continue
			}

			// Check whether having already endorsed the same leader in this round
			if time, ok := endorsedLeader.Get(leader); ok {
				if time.(uint64) == n.cons.Timestamp(now) {
					debugLog("reject bs from the same leader", "id", bs.ID().Abev())
					continue
				}
			}

			debugLog("<== bs", "id", bs.ID().Abev())

			if err := n.cons.ValidateBlockSummary(bs, best.Header(), now); err != nil {
				debugLog("invalid bs", "err", err, "id", bs.ID().Abev())
				continue
			}

			// Check the committee membership
			ok, proof, err := n.cons.IsCommittee(n.master.VrfPrivateKey, now)
			if err != nil {
				errLog("check committee", "err", err)
				continue
			}
			if ok {
				// Endorse the block summary
				ed := block.NewEndorsement(bs, proof)
				sig, err := crypto.Sign(ed.SigningHash().Bytes(), n.master.PrivateKey)
				if err != nil {
					errLog("Signing endorsement", "err", err)
					continue
				}
				ed = ed.WithSignature(sig)

				// update cache
				endorsedLeader.Set(leader, bs.Timestamp())

				debugLog("ed ==>", "id", ed.ID().Abev())
				// if the node is the leader, send the endorsement back to packerLoop
				// for packing block and to housekeeping for broadcasting
				if leader == n.master.Address() {
					n.comm.SendEndorsementToFeed(ed)
				} else {
					n.comm.BroadcastEndorsement(ed)
				}
			}
		}
	}
}

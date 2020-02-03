package node

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/thor"
)

func randByte32() (b thor.Bytes32) {
	rand.Read(b[:])
	return
}

func (n *Node) sendBlockSummary() {
	bs := block.NewBlockSummary(
		randByte32(),
		randByte32(),
		uint64(0),
		uint64(0),
	)
	sig, _ := crypto.Sign(bs.SigningHash().Bytes(), n.master.PrivateKey)
	bs = bs.WithSignature(sig)

	log.Debug("sent block summary", "status", "valid", "id", bs.ID())
	n.comm.BroadcastBlockSummary(bs)

	bs = bs.Copy().WithSignature([]byte(nil))
	if !bs.ID().IsZero() {
		panic("id should be zero")
	}
	log.Debug("sent block summary", "status", "invalid", "id", bs.ID())
	n.comm.BroadcastBlockSummary(bs)
}

func (n *Node) simpleHouseKeeping(ctx context.Context) {
	log.Debug("enter test house keeping")
	defer log.Debug("leave test house keeping")

	var scope event.SubscriptionScope
	defer scope.Close()

	newBlockCh := make(chan *comm.NewBlockEvent)
	newBlockSummaryCh := make(chan *comm.NewBlockSummaryEvent)
	newEndorsementCh := make(chan *comm.NewEndorsementEvent)
	newTxSetCh := make(chan *comm.NewTxSetEvent)
	newBlockHeaderCh := make(chan *comm.NewBlockHeaderEvent)

	scope.Track(n.comm.SubscribeBlock(newBlockCh))
	scope.Track(n.comm.SubscribeBlockSummary(newBlockSummaryCh))
	scope.Track(n.comm.SubscribeEndorsement(newEndorsementCh))
	scope.Track(n.comm.SubscribeTxSet(newTxSetCh))
	scope.Track(n.comm.SubscribeBlockHeader(newBlockHeaderCh))

	ticker := time.NewTicker(time.Duration(thor.BlockInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case dat := <-newBlockSummaryCh:
			log.Debug("received block summary", "id", dat.ID())
			n.comm.BroadcastBlockSummary(dat.Summary)
		case dat := <-newTxSetCh:
			log.Debug("received tx set", "id", dat.ID())
			n.comm.BroadcastTxSet(dat.TxSet)
		case dat := <-newEndorsementCh:
			log.Debug("received endorsement", "id", dat.ID())
			n.comm.BroadcastEndorsement(dat.Endorsement)
		case dat := <-newBlockHeaderCh:
			log.Debug("received block header", "id", dat.ID())
			n.comm.BroadcastBlockHeader(dat.Header)
		}
	}
}

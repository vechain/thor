package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/thor"
)

func randByte32() (b thor.Bytes32) {
	rand.Read(b[:])
	return
}

func (n *Node) sendNewStructObj(ctx context.Context) {
	addr, _ := hex.DecodeString("475f483a6cba6b0eba8ff269aa9d4fcd5b5275e3")
	if bytes.Compare(n.master.Address().Bytes(), addr) != 0 {
		log.Debug("exit sending loop, key not matched")
		return
	}

	log.Info("starting sending loop")
	defer func() { log.Info("existing sending loop") }()

	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	<-ticker.C
	// b := new(block.Builder).GasLimit(10000).Timestamp(1231090).Build()
	// log.Info("sent block", "id", b.Header().ID())
	// n.comm.BroadcastBlock(b)

	bs := block.RandBlockSummary()
	log.Info("sent block summary", "id", bs.ID())
	n.comm.BroadcastBlockSummary(bs)

	ed := block.RandEndorsement(block.RandBlockSummary())
	log.Info("send endoresement", "id", ed.ID())
	n.comm.BroadcastEndorsement(ed)

	ts := block.RandTxSet(10)
	log.Info("sent tx set", "id", ts.ID())
	n.comm.BroadcastTxSet(ts)

	header := block.RandBlockHeader()
	log.Info("sending block header", "id", header.ID())
	n.comm.BroadcastBlockHeader(header)
}

func (n *Node) simpleHouseKeeping(ctx context.Context) {
	log.Info("enter test house keeping")
	defer log.Info("leave test house keeping")

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
		case dat := <-newBlockCh:
			log.Info("received block", "id", dat.Header().ID())
			n.comm.BroadcastBlock(dat.Block)
		case dat := <-newBlockSummaryCh:
			log.Info("received block summary", "id", dat.ID())
			n.comm.BroadcastBlockSummary(dat.Summary)
		case dat := <-newTxSetCh:
			log.Info("received tx set", "id", dat.ID())
			n.comm.BroadcastTxSet(dat.TxSet)
		case dat := <-newEndorsementCh:
			log.Info("received endorsement", "id", dat.ID())
			n.comm.BroadcastEndorsement(dat.Endorsement)
		case dat := <-newBlockHeaderCh:
			log.Info("received block header", "id", dat.ID())
			n.comm.BroadcastBlockHeader(dat.Header)
		}
	}
}

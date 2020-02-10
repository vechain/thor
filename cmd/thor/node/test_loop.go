package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

func randByte32() (b thor.Bytes32) {
	rand.Read(b[:])
	return
}

func (n *Node) sendNewStructObj(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 20)
	defer ticker.Stop()

	<-ticker.C

	bs := block.RandBlockSummary()
	log.Debug("sending new block summary", "id", bs.ID())
	n.comm.BroadcastBlockSummary(bs)

	ed := block.RandEndorsement(block.RandBlockSummary())
	log.Debug("sending new endoresement", "id", ed.ID())
	n.comm.BroadcastEndorsement(ed)

	ts := block.RandTxSet(10)
	log.Debug("sending new tx set", "id", ts.ID())
	n.comm.BroadcastTxSet(ts)

	header := block.RandBlockHeader()
	log.Debug("sending new block header", "id", header.ID())
	n.comm.BroadcastBlockHeader(header)
}

func (n *Node) sendBlockComponets() {
	addr, _ := hex.DecodeString("")
	if bytes.Compare(n.master.Address().Bytes(), addr) != 0 {
		return
	}

	best := n.chain.BestBlock()

	flow, err := n.packer.Schedule(best.Header(), uint64(time.Now().Unix()))
	if err != nil {
		log.Error("Schedule", "err", err)
		return
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-ticker.C:
			now := uint64(time.Now().Unix())
			if now < flow.When() {
				continue
			} else {
				break loop
			}
		}
	}

	log.Debug("begin to pack")
	t1 := mclock.Now()

	bs, _, err := flow.PackTxSetAndBlockSummary(n.master.PrivateKey)
	log.Debug("packed block summary", "id", bs.ID())

	ethsk, _ := crypto.HexToECDSA("")
	_, vrfsk := vrf.GenKeyPairFromSeed(ethsk.D.Bytes())

}

func (n *Node) simpleHouseKeeping(ctx context.Context) {
	log.Debug("entering simple house-keeping loop")
	defer log.Info("leaving simple house-keeping loop")

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
			log.Debug("received new block", "id", dat.Header().ID())
			n.comm.BroadcastBlock(dat.Block)
		case dat := <-newBlockSummaryCh:
			log.Debug("received new block summary", "id", dat.ID())
			n.comm.BroadcastBlockSummary(dat.Summary)
		case dat := <-newTxSetCh:
			log.Debug("received new tx set", "id", dat.ID())
			n.comm.BroadcastTxSet(dat.TxSet)
		case dat := <-newEndorsementCh:
			log.Debug("received new endorsement", "id", dat.ID())
			n.comm.BroadcastEndorsement(dat.Endorsement)
		case dat := <-newBlockHeaderCh:
			log.Debug("received new block header", "id", dat.ID())
			n.comm.BroadcastBlockHeader(dat.Header)
		}
	}
}

package node

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
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

// testBroadcasting randomly creates and broadcast instances of
// the structs defined for vip193. It is used for testing the
// sending/receiving functions.
func (n *Node) testBroadcasting(ctx context.Context) {
	ticker := time.NewTicker(time.Second * 10)
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

func (n *Node) testEmptyBlockAssembling(ctx context.Context) {
	// Ubuntu
	hexKeys := []string{
		"9394eda09b27bba53362d88c1c7aac18463468492b86e0ff7a6aa9bfd9753bd5",
		"0276397acb72009048bcf3e91fda656fc87511b685f28b181e620817b1806e71",
		"19e4a1bb4ccd861ba4aedb6c74f4b94165d2dbb4eea6acc9a701bfb5b6adc843",
	}

	var (
		ethsks []*ecdsa.PrivateKey
		vrfsks []*vrf.PrivateKey
	)

	for _, key := range hexKeys {
		ethsk, _ := crypto.HexToECDSA(key)
		ethsks = append(ethsks, ethsk)
		_, vrfsk := vrf.GenKeyPairFromSeed(ethsk.D.Bytes())
		vrfsks = append(vrfsks, vrfsk)
	}

	if bytes.Compare(n.master.Address().Bytes(), crypto.PubkeyToAddress(ethsks[0].PublicKey).Bytes()) != 0 {
		return
	}

	best := n.chain.BestBlock()
	flow, err := n.packer.Schedule(best.Header(), uint64(time.Now().Unix()))
	if err != nil {
		log.Error("Schedule", "err", err)
		return
	}

	<-time.NewTicker(time.Second * 20).C

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := uint64(time.Now().Unix())
			if now < flow.When() {
				continue
			} else {
				break loop
			}
		}
	}

	log.Info("packing new block")
	launchTime := mclock.Now()

	bs, ts, err := flow.PackTxSetAndBlockSummary(n.master.PrivateKey)
	summaryDone := mclock.Now()

	for i, vrfsk := range vrfsks {
		ok, proof, err := n.cons.IsCommittee(vrfsk, uint64(time.Now().Unix()))
		if err != nil || !ok {
			panic("Not a committee")
		}

		ed := block.NewEndorsement(bs, proof)
		sig, _ := crypto.Sign(ed.SigningHash().Bytes(), ethsks[i])
		ed = ed.WithSignature(sig)

		flow.AddEndoresement(ed)
	}
	endorsementDone := mclock.Now()

	block, stage, receipts, err := flow.Pack(n.master.PrivateKey)
	blockDone := mclock.Now()

	log.Debug("Committing new block")
	if err := n.commit(block, stage, receipts); err != nil {
		panic(err)
	}
	commitDone := mclock.Now()

	display(block, receipts,
		summaryDone-launchTime,
		endorsementDone-summaryDone,
		blockDone-endorsementDone,
		commitDone-blockDone,
	)

	log.Info("broadcasting block header", "id", block.Header().ID())
	n.comm.BroadcastBlockHeader(block.Header())
	if ts.IsEmpty() {
		log.Info("broadcasting tx set", "id", ts.ID())
		n.comm.BroadcastTxSet(ts)
	}
}

func (n *Node) simpleHouseKeeping(ctx context.Context) {
	log.Info("entering simple house-keeping loop")
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

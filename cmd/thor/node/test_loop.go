package node

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"runtime"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
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
	ticker := time.NewTicker(time.Second * 20)
	defer ticker.Stop()

	<-ticker.C

	bs := block.RandBlockSummary()
	log.Info("sending new block summary", "id", bs.ID())
	n.comm.BroadcastBlockSummary(bs)

	ed := block.RandEndorsement(block.RandBlockSummary())
	log.Info("sending new endoresement", "id", ed.ID())
	n.comm.BroadcastEndorsement(ed)

	ts := block.RandTxSet(10)
	log.Info("sending new tx set", "id", ts.ID())
	n.comm.BroadcastTxSet(ts)

	header := block.RandBlockHeader()
	log.Info("sending new block header", "id", header.ID())
	n.comm.BroadcastBlockHeader(header)
}

func getKeys() (ethsks []*ecdsa.PrivateKey, vrfsks []*vrf.PrivateKey) {
	var hexKeys []string

	switch runtime.GOOS {
	case "linux":
		hexKeys = []string{
			"ebe662faa74cd42422ff0374690798d22d00c2f27cd478ebe43f129bdb53c15c",
			"0276397acb72009048bcf3e91fda656fc87511b685f28b181e620817b1806e71",
			"19e4a1bb4ccd861ba4aedb6c74f4b94165d2dbb4eea6acc9a701bfb5b6adc843",
		}
	case "darwin":
		hexKeys = []string{
			// "ebe662faa74cd42422ff0374690798d22d00c2f27cd478ebe43f129bdb53c15c",
			// "b59e57175c45c85463ac948cdfc7f669e70922d3c6ae56843022dac76855f552",
			// "a0ca961e7e98ff17b2593195c39e4bc21472c29f215ac056930cbd7b06084a27",
		}
	default:
		panic("unrecognized os")
	}

	for _, key := range hexKeys {
		ethsk, _ := crypto.HexToECDSA(key)
		ethsks = append(ethsks, ethsk)
		_, vrfsk := vrf.GenKeyPairFromSeed(ethsk.D.Bytes())
		vrfsks = append(vrfsks, vrfsk)
	}

	return
}

func emptyLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		}
	}
}

func (n *Node) newLocalBlock(ctx context.Context,
	ethsks []*ecdsa.PrivateKey, vrfsks []*vrf.PrivateKey,
	txs tx.Transactions) (
	blk *block.Block, stage *state.Stage, receipts tx.Receipts, err error,
) {
	best := n.chain.BestBlock()
	flow, err := n.packer.Schedule(best.Header(), uint64(time.Now().Unix()))
	if err != nil {
		return
	}

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

	for _, tx := range txs {
		if err := flow.Adopt(tx); err != nil {
			log.Warn("failed to add tx", "id", tx.ID())
		}
	}
	bs, _, err := flow.PackTxSetAndBlockSummary(n.master.PrivateKey)

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

	blk, stage, receipts, err = flow.Pack(n.master.PrivateKey)
	return
}

func (n *Node) testSync(ctx context.Context) {
	ethsks, vrfsks := getKeys()
	// exit if it is not node 1
	if bytes.Compare(n.master.Address().Bytes(), crypto.PubkeyToAddress(ethsks[0].PublicKey).Bytes()) != 0 {
		return
	}

	<-time.NewTimer(time.Second * 10).C

	blk, _, _, err := n.newLocalBlock(ctx, ethsks, vrfsks, nil)
	if err != nil {
		panic(err)
	}

	var stats blockStats
	isTrunk, err := n.processBlock(blk, &stats)
	if err != nil {
		panic(err)
	}

	log.Info("added new block", "id", blk.Header().ID(), "num", blk.Header().Number())

	if isTrunk {
		log.Info("broadcast block id", "id", blk.Header().ID())
		n.comm.BroadcastBlockID(blk.Header().ID())
	} else {
		panic("not trunk")
	}
}

func (n *Node) testEmptyBlockAssembling(ctx context.Context) {
	ethsks, vrfsks := getKeys()
	// exit if it is not node 2
	if bytes.Compare(n.master.Address().Bytes(), crypto.PubkeyToAddress(ethsks[1].PublicKey).Bytes()) != 0 {
		return
	}

	<-time.NewTimer(time.Second * 10).C

	blk, _, _, err := n.newLocalBlock(ctx, ethsks, vrfsks, nil)
	if err != nil {
		panic(err)
	}

	isTrunk, err := n.processBlock(blk, new(blockStats))
	if err != nil {
		panic(err)
	}

	log.Info("created new block", "id", blk.Header().ID())

	if isTrunk {
		log.Info("broad new block header", "id", blk.Header().ID())
		n.comm.BroadcastBlockHeader(blk.Header())
	} else {
		panic("not trunk")
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
			log.Info("received new block", "id", dat.Header().ID())
			n.comm.BroadcastBlock(dat.Block)
		case dat := <-newBlockSummaryCh:
			log.Info("received new block summary", "id", dat.ID())
			n.comm.BroadcastBlockSummary(dat.Summary)
		case dat := <-newTxSetCh:
			log.Info("received new tx set", "id", dat.ID())
			n.comm.BroadcastTxSet(dat.TxSet)
		case dat := <-newEndorsementCh:
			log.Info("received new endorsement", "id", dat.ID())
			n.comm.BroadcastEndorsement(dat.Endorsement)
		case dat := <-newBlockHeaderCh:
			log.Info("received new block header", "id", dat.ID())
			n.comm.BroadcastBlockHeader(dat.Header)
		}
	}
}

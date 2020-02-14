package node

import (
	"context"
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	mathrand "math/rand"
	"runtime"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/comm"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vrf"
)

func randByte32() (b thor.Bytes32) {
	cryptorand.Read(b[:])
	return
}

// testra ==>ndomly creates and broadcast instances of
// the structs defined for vip193. It is used for testing the
// sending/receiving functions.
func (n *Node) testCase2(ctx context.Context) {
	<-time.After(time.Second * 10)

	bs := block.RandBlockSummary()
	log.Info("bs ==>", "id", bs.ID())
	n.comm.BroadcastBlockSummary(bs)

	ed := block.RandEndorsement(block.RandBlockSummary())
	log.Info("ed ==>", "id", ed.ID())
	n.comm.BroadcastEndorsement(ed)

	ts := block.RandTxSet(10)
	log.Info("ts ==>", "id", ts.ID())
	n.comm.BroadcastTxSet(ts)

	header := block.RandBlockHeader()
	log.Info("hd ==>", "id", header.ID())
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
			// "a0ca961e7e98ff17b2593195c39e414 ==>72c29f215ac056930cbd7b06084a27",
		}
	default:
		panic("unrecognized os")
	}

	for _, key := range hexKeys {
		ethsk, err := crypto.HexToECDSA(key)
		if err != nil {
			panic(err)
		}
		ethsks = append(ethsks, ethsk)
		_, vrfsk := vrf.GenKeyPairFromSeed(ethsk.D.Bytes())
		vrfsks = append(vrfsks, vrfsk)
	}

	return
}

func (n *Node) getNodeID() int {
	ethsks, _ := getKeys()
	for i, sk := range ethsks {
		addr := thor.BytesToAddress(crypto.PubkeyToAddress(sk.PublicKey).Bytes())
		if addr == n.master.Address() {
			return i + 1
		}
	}
	panic("node not found")
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

func (n *Node) newLocalBsTs(ctx context.Context, txs tx.Transactions) (
	bs *block.Summary, ts *block.TxSet, flow *packer.Flow) {

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
			panic(errors.WithMessage(err, "failed to adopt the tx"))
		}
	}
	bs, ts, err = flow.PackTxSetAndBlockSummary(n.master.PrivateKey)
	if err != nil {
		panic(err)
	}

	return
}

func (n *Node) newLocalBlock(
	ctx context.Context,
	ethsks []*ecdsa.PrivateKey, vrfsks []*vrf.PrivateKey,
	txs tx.Transactions,
) (blk *block.Block, stage *state.Stage, receipts tx.Receipts, bs *block.Summary, ts *block.TxSet) {

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
			panic(errors.WithMessage(err, "failed to adopt the tx"))
		}
	}
	bs, ts, err = flow.PackTxSetAndBlockSummary(n.master.PrivateKey)

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

func (n *Node) testCase3(ctx context.Context) {
	ethsks, vrfsks := getKeys()
	// // exit if it is not node 1
	// if bytes.Compare(n.master.Address().Bytes(), crypto.PubkeyToAddress(ethsks[0].PublicKey).Bytes()) != 0 {
	// 	return
	// }

	<-time.After(time.Second * 10)

	blk, _, _, _, _ := n.newLocalBlock(ctx, ethsks, vrfsks, nil)

	var stats blockStats
	isTrunk, err := n.processBlock(blk, &stats)
	if err != nil {
		panic(err)
	}

	log.Info("added new block", "id", blk.Header().ID(), "num", blk.Header().Number())

	if isTrunk {
		log.Info("bl ==>ock id", "id", blk.Header().ID())
		n.comm.BroadcastBlockID(blk.Header().ID())
	} else {
		panic("not trunk")
	}
}

func (n *Node) testCase4(ctx context.Context) {
	ethsks, vrfsks := getKeys()
	// // exit if it is not node 2
	// if bytes.Compare(n.master.Address().Bytes(), crypto.PubkeyToAddress(ethsks[1].PublicKey).Bytes()) != 0 {
	// 	return
	// }

	<-time.After(time.Second * 10)

	blk, _, _, _, _ := n.newLocalBlock(ctx, ethsks, vrfsks, nil)

	isTrunk, err := n.processBlock(blk, new(blockStats))
	if err != nil {
		panic(err)
	}

	log.Info("created new block", "id", blk.Header().ID())

	if isTrunk {
		log.Info("hd ==>", "id", blk.Header().ID())
		n.comm.BroadcastBlockHeader(blk.Header())
	} else {
		panic("not trunk")
	}
}

func (n *Node) testCase5(ctx context.Context) {
	ethsks, vrfsks := getKeys()
	// // exit if it is not node 2
	// if bytes.Compare(n.master.Address().Bytes(), crypto.PubkeyToAddress(ethsks[1].PublicKey).Bytes()) != 0 {
	// 	return
	// }

	<-time.After(time.Second * 10)

	var txs tx.Transactions
	for i := 0; i < 5; i++ {
		sk, _ := crypto.GenerateKey()
		addr := n.master.Address()

		tx := new(tx.Builder).
			Clause(tx.NewClause(&addr)).
			Gas(21000).
			ChainTag(n.chain.Tag()).
			Expiration(100).
			Nonce(mathrand.Uint64()).
			Build()
		sig, _ := crypto.Sign(tx.SigningHash().Bytes(), sk)
		tx = tx.WithSignature(sig)
		txs = append(txs, tx)
	}

	blk, _, _, _, ts := n.newLocalBlock(ctx, ethsks, vrfsks, txs)

	isTrunk, err := n.processBlock(blk, new(blockStats))
	if err != nil {
		panic(err)
	}

	log.Info("created new block", "id", blk.Header().ID())

	if isTrunk {
		log.Info("ts ==>", "id", ts.ID())
		n.comm.BroadcastTxSet(ts)

		<-time.After(time.Second)

		log.Info("hd ==>", "id", blk.Header().ID())
		n.comm.BroadcastBlockHeader(blk.Header())
	} else {
		panic("not trunk")
	}
}

func (n *Node) testCase6(ctx context.Context) {
	ethsks, vrfsks := getKeys()
	// // exit if it is not node 2
	// if bytes.Compare(n.master.Address().Bytes(), crypto.PubkeyToAddress(ethsks[1].PublicKey).Bytes()) != 0 {
	// 	return
	// }

	<-time.After(time.Second * 10)

	var txs tx.Transactions
	for i := 0; i < 5; i++ {
		sk, _ := crypto.GenerateKey()
		addr := n.master.Address()

		tx := new(tx.Builder).
			Clause(tx.NewClause(&addr)).
			Gas(21000).
			ChainTag(n.chain.Tag()).
			Expiration(100).
			Nonce(mathrand.Uint64()).
			Build()
		sig, _ := crypto.Sign(tx.SigningHash().Bytes(), sk)
		tx = tx.WithSignature(sig)
		txs = append(txs, tx)
	}

	blk, _, _, _, ts := n.newLocalBlock(ctx, ethsks, vrfsks, txs)

	isTrunk, err := n.processBlock(blk, new(blockStats))
	if err != nil {
		panic(err)
	}

	log.Info("created new block", "id", blk.Header().ID())

	if isTrunk {
		log.Info("hd ==>", "id", blk.Header().ID())
		n.comm.BroadcastBlockHeader(blk.Header())

		<-time.After(time.Second)

		log.Info("ts ==>", "id", ts.ID())
		n.comm.BroadcastTxSet(ts)
	} else {
		panic("not trunk")
	}
}

func (n *Node) testCase7(ctx context.Context) {
	<-time.After(time.Second * 10)

	bs, _, _ := n.newLocalBsTs(ctx, nil)
	log.Info("bs ==>", "id", bs.ID())
	n.comm.BroadcastBlockSummary(bs)

	<-time.After(time.Second)

	bs = block.NewBlockSummary(bs.ParentID(), randByte32(), bs.Timestamp(), bs.TotalScore())
	sig, _ := crypto.Sign(bs.SigningHash().Bytes(), n.master.PrivateKey)
	bs = bs.WithSignature(sig)
	log.Info("bs ==>", "id", bs.ID())
	n.comm.BroadcastBlockSummary(bs)
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
			log.Info("<== blk", "id", dat.Header().ID())
			n.comm.BroadcastBlock(dat.Block)
		case dat := <-newBlockSummaryCh:
			log.Info("<== bs", "id", dat.ID())
			n.comm.BroadcastBlockSummary(dat.Summary)
		case dat := <-newTxSetCh:
			log.Info("<== ts", "id", dat.ID())
			n.comm.BroadcastTxSet(dat.TxSet)
		case dat := <-newEndorsementCh:
			log.Info("<== ed", "id", dat.ID())
			n.comm.BroadcastEndorsement(dat.Endorsement)
		case dat := <-newBlockHeaderCh:
			log.Info("<== hd", "id", dat.ID())
			n.comm.BroadcastBlockHeader(dat.Header)
		}
	}
}

func (n *Node) endorserLoop(ctx context.Context) {
	log.Info("started endorsement loop")
	defer log.Info("leaving endorsement loop")

	log.Info("waiting for synchronization...")
	select {
	case <-ctx.Done():
		return
	case <-n.comm.Synced():
	}
	log.Info("synchronization process done")

	var scope event.SubscriptionScope
	defer scope.Close()

	newBlockSummaryCh := make(chan *comm.NewBlockSummaryEvent)
	scope.Track(n.comm.SubscribeBlockSummary(newBlockSummaryCh))

	var lbs *block.Summary

	for {
		select {
		case <-ctx.Done():
			return

		case ev := <-newBlockSummaryCh:
			bs := ev.Summary
			log.Info("<== bs", "key", "eder", "id", bs.ID())

			now := uint64(time.Now().Unix())
			best := n.chain.BestBlock()

			// Only receive one block summary from the same leader once in the same round
			if lbs != nil {
				if n.cons.ValidateBlockSummary(lbs, best.Header(), now) == nil {
					log.Info("bs rejected", "key", "eder", "id", bs.ID())
					continue
				}
				lbs = nil
			}

			if err := n.cons.ValidateBlockSummary(bs, best.Header(), now); err != nil {
				panic(errors.WithMessage(err, "invalid bs"))
			}

			lbs = bs

			// Check the committee membership
			ok, proof, err := n.cons.IsCommittee(n.master.VrfPrivateKey, now)
			if err != nil {
				panic(errors.WithMessage(err, "error when checking committee membership"))
			}
			if ok {
				// Endorse the block summary
				ed := block.NewEndorsement(bs, proof)
				sig, _ := crypto.Sign(ed.SigningHash().Bytes(), n.master.PrivateKey)
				ed = ed.WithSignature(sig)

				log.Info("ed ==>", "key", "eder", "id", ed.ID())
				n.comm.BroadcastEndorsement(ed)
			} else {
				panic("not a committee member")
			}
		}
	}
}

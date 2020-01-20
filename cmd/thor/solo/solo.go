// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/event"
	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cmd/thor/bandwidth"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
	"github.com/vechain/thor/vrf"
)

var log = log15.New("pkg", "solo")

// Solo mode is the standalone client without p2p server
type Solo struct {
	repo        *chain.Repository
	txPool      *txpool.TxPool
	packer      *packer.Packer
	logDB       *logdb.LogDB
	bestBlockCh chan *block.Block
	gasLimit    uint64
	bandwidth   bandwidth.Bandwidth
	onDemand    bool
	skipLogs    bool

	cons *consensus.Consensus
	sk   *ecdsa.PrivateKey
}

// New returns Solo instance
func New(
	repo *chain.Repository,
	stater *state.Stater,
	logDB *logdb.LogDB,
	txPool *txpool.TxPool,
	gasLimit uint64,
	onDemand bool,
	skipLogs bool,
	forkConfig thor.ForkConfig,
) *Solo {
	return &Solo{
		repo:   repo,
		txPool: txPool,
		packer: packer.New(
			repo,
			stater,
			genesis.DevAccounts()[0].Address,
			&genesis.DevAccounts()[0].Address,
			forkConfig),
		logDB:    logDB,
		gasLimit: gasLimit,
		skipLogs: skipLogs,
		onDemand: onDemand,
		cons:     consensus.New(chain, stateCreator, forkConfig),
	}
}

// Run runs the packer for solo
func (s *Solo) Run(ctx context.Context) error {
	goes := &co.Goes{}

	defer func() {
		<-ctx.Done()
		goes.Wait()
	}()

	goes.Go(func() {
		s.loop(ctx)
	})

	log.Info("prepared to pack block")

	return nil
}

func (s *Solo) loop(ctx context.Context) {
	tickerBegin := time.NewTicker(time.Duration(10) * time.Second)
	defer tickerBegin.Stop()
	// tickerStop := time.NewTicker(time.Duration(5) * time.Second)
	// defer tickerStop.Stop()

	var scope event.SubscriptionScope
	defer scope.Close()

	// bsCh := make(chan *block.Summary)
	// edCh := make(chan *block.Endorsement)
	// tsCh := make(chan *block.TxSet)
	// hdCh := make(chan *block.Header)

	// var (
	// 	bs   blockSummary
	// 	ts   txSet
	// eds endorsements
	// 	flow *packer.Flow
	// 	err  error
	// )

	// txEvCh := make(chan *txpool.TxEvent, 10)
	// scope.Track(s.txPool.SubscribeTxEvent(txEvCh))

	if err := s.packing(nil); err != nil {
		log.Error("failed to pack block", "err", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("stopping packing service......")
			return

		case <-tickerBegin.C:
			log.Debug("START")
			startTime := mclock.Now()

			var (
				bs   *block.Summary
				ts   *block.TxSet
				err  error
				flow *packer.Flow

				header   *block.Header
				stage    *state.Stage
				receipts tx.Receipts
			)

			best := s.chain.BestBlock()
			txs := s.txPool.Executables()
			flow, err = s.packer.Mock(best.Header(), uint64(time.Now().Unix()), s.gasLimit)
			if err != nil {
				log.Error("packer.Mock", "error", err)
			}

			doneCh1 := make(chan struct{})
			go func() {
				time.Sleep(time.Duration(3) * time.Second)
				close(doneCh1)
			}()
			bs, ts, err = s.packTxSetAndBlockSummary(doneCh1, flow, txs)
			if err != nil {
				log.Debug("packTxSetAndBlockSummary", "error", err)
			}

			log.Debug("Endorsing starts")
			doneCh2 := make(chan struct{})
			edCh := make(chan *block.Endorsement, thor.CommitteeSize)
			for i := uint64(0); i < thor.CommitteeSize*2; i++ {
				go s.endorse(doneCh2, edCh, bs)
			}

			var eds block.Endorsements
			for i := uint64(0); i < thor.CommitteeSize*2; i++ {
				if eds.Len() >= int(thor.CommitteeSize) {
					close(doneCh2)
					break
				}
				select {
				case ed := <-edCh:
					if eds.Add(ed) {
						log.Debug("Collecting endorsement", "#endorsement", eds.Len())
					}
				}
			}
			log.Debug("Endorsing ends")

			header, stage, receipts, err = flow.PackHeader(
				genesis.DevAccounts()[0].PrivateKey,
				[]uint8{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
				eds.VrfProofs(),
				bs.Signature(),
				eds.Signatures(),
			)
			if err != nil {
				log.Error("flow.PackHeader", "error", err)
			}

			execElapsed := mclock.Now() - startTime

			b := block.Compose(header, ts.Transactions())
			err = s.commit(b, stage, receipts)
			if err != nil {
				log.Error("s.pack", "error", err)
			}

			commitElapsed := mclock.Now() - execElapsed - startTime

			display(b, receipts, execElapsed, commitElapsed)
		}
	}
}

func display(b *block.Block, receipts tx.Receipts, execElapsed, commitElapsed mclock.AbsTime) {
	blockID := b.Header().ID()
	log.Info("📦 new block packed",
		"txs", len(receipts),
		"mgas", float64(b.Header().GasUsed())/1000/1000,
		"et", fmt.Sprintf("%v|%v", common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
		"id", fmt.Sprintf("[#%v…%x]", block.Number(blockID), blockID[28:]),
	)
	log.Debug(b.String())
}

func (s *Solo) commit(b *block.Block, stage *state.Stage, receipts tx.Receipts) error {
	if _, err := stage.Commit(); err != nil {
		return errors.WithMessage(err, "commit state")
	}

	// ignore fork when solo
	_, err := s.chain.AddBlock(b, receipts)
	if err != nil {
		return errors.WithMessage(err, "commit block")
	}

	task := s.logDB.NewTask().ForBlock(b.Header())
	for i, tx := range b.Transactions() {
		origin, _ := tx.Origin()
		task.Write(tx.ID(), origin, receipts[i].Outputs)
	}
	if err := task.Commit(); err != nil {
		return errors.WithMessage(err, "commit log")
	}

	return nil
}

func (s *Solo) endorse(done chan struct{}, edCh chan *block.Endorsement, bs *block.Summary) {
	endorseHash := bs.EndorseHash()

	for i := uint64(0); i < thor.CommitteeSize*2; i++ {
		_, sk := vrf.GenKeyPair()
		proof, _ := sk.Prove(endorseHash.Bytes())
		if consensus.IsCommitteeByProof(proof) {
			ed := block.NewEndorsement(bs, proof)
			sig, _ := crypto.Sign(ed.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
			ed = ed.WithSignature(sig)

			select {
			case <-done:
				log.Debug("endorsement done")
				return
			case edCh <- ed:
				log.Debug("endorsement sent", "id", hex.EncodeToString(ed.SigningHash().Bytes()))
				return
			}
		}
	}
}

func (s *Solo) packTxSetAndBlockSummary(done chan struct{}, flow *packer.Flow, txs tx.Transactions) (*block.Summary, *block.TxSet, error) {
	best := s.chain.BestBlock()

	var txsToRemove []*tx.Transaction
	defer func() {
		for _, tx := range txsToRemove {
			s.txPool.Remove(tx.Hash(), tx.ID())
		}
	}()
	for _, tx := range txs {
		select {
		case <-done:
			break
		}
		err := flow.Adopt(tx)
		switch {
		case packer.IsGasLimitReached(err):
			break
		case packer.IsTxNotAdoptableNow(err):
			continue
		default:
			txsToRemove = append(txsToRemove, tx)
		}
	}

	ts := block.NewTxSet(flow.Txs())
	sig, err := crypto.Sign(ts.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		return nil, nil, err
	}
	ts = ts.WithSignature(sig)

	parent := best.Header().ID()
	root := flow.Txs().RootHash()
	time := best.Header().Timestamp() + thor.BlockInterval
	bs := block.NewBlockSummary(parent, root, time)
	sig, err = crypto.Sign(bs.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		return nil, nil, err
	}
	bs = bs.WithSignature(sig)

	return bs, ts, nil
}

func (s *Solo) packing(pendingTxs tx.Transactions) error {
	best := s.repo.BestBlock()
	var txsToRemove []*tx.Transaction
	defer func() {
		for _, tx := range txsToRemove {
			s.txPool.Remove(tx.Hash(), tx.ID())
		}
	}()

	if s.gasLimit == 0 {
		suggested := s.bandwidth.SuggestGasLimit()
		s.packer.SetTargetGasLimit(suggested)
	}

	flow, err := s.packer.Mock(best.Header(), uint64(time.Now().Unix()), s.gasLimit)
	if err != nil {
		return errors.WithMessage(err, "mock packer")
	}

	startTime := mclock.Now()
	for _, tx := range pendingTxs {
		err := flow.Adopt(tx)
		switch {
		case packer.IsGasLimitReached(err):
			break
		case packer.IsTxNotAdoptableNow(err):
			continue
		default:
			txsToRemove = append(txsToRemove, tx)
		}
	}

	b, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		return errors.WithMessage(err, "pack")
	}
	execElapsed := mclock.Now() - startTime

	// If there is no tx packed in the on-demand mode then skip
	if s.onDemand && len(b.Transactions()) == 0 {
		return nil
	}

	if _, err := stage.Commit(); err != nil {
		return errors.WithMessage(err, "commit state")
	}

	// ignore fork when solo
	if err := s.repo.AddBlock(b, receipts); err != nil {
		return errors.WithMessage(err, "commit block")
	}
	if err := s.repo.SetBestBlockID(b.Header().ID()); err != nil {
		return errors.WithMessage(err, "set best block")
	}

	if !s.skipLogs {
		if err := s.logDB.Log(func(w *logdb.Writer) error {
			return w.Write(b, receipts)
		}); err != nil {
			return errors.WithMessage(err, "commit log")
		}
	}

	commitElapsed := mclock.Now() - startTime - execElapsed

	if v, updated := s.bandwidth.Update(b.Header(), time.Duration(execElapsed+commitElapsed)); updated {
		log.Debug("bandwidth updated", "gps", v)
	}

	blockID := b.Header().ID()
	log.Info("📦 new block packed",
		"txs", len(receipts),
		"mgas", float64(b.Header().GasUsed())/1000/1000,
		"et", fmt.Sprintf("%v|%v", common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
		"id", fmt.Sprintf("[#%v…%x]", block.Number(blockID), blockID[28:]),
	)
	log.Debug(b.String())

	return nil
}

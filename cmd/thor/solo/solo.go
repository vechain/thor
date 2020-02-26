// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/crypto"
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
		cons:     consensus.New(repo, stater, forkConfig),
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
	ticker := time.NewTicker(time.Duration(10) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("stopping packing service......")
			return

		case <-ticker.C:
			startTime := mclock.Now()

			var (
				err  error
				flow *packer.Flow
				// done chan struct{}
			)

			best := s.repo.BestBlock()
			flow, err = s.packer.Mock(best.Header(), uint64(time.Now().Unix()), s.gasLimit)
			if err != nil {
				log.Error("packer.Mock", "error", err)
			}

			if err := s.packTxSetAndBlockSummary(flow, 3); err != nil {
				log.Error("PackTxSetAndBlockSummary", "err", err)
				continue
			}

			prepareElapsed := mclock.Now() - startTime

			// Produce endorsements that are signed by the block producer
			// but include random generated VRF proofs
			bs := flow.GetBlockSummary()
			seed := bs.ID()
			for flow.NumOfEndorsements() < int(thor.CommitteeSize) {
				// generate vrf proofs from random vrf private keys
				_, sk := vrf.GenKeyPair()
				proof, err := sk.Prove(seed.Bytes())
				if err != nil {
					log.Error("failed to produce vrf proof")
					continue
				}

				ed := block.NewEndorsement(bs, proof)
				sig, err := crypto.Sign(ed.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
				if err != nil {
					log.Error("failed to sign the edorsement")
					continue
				}
				ed = ed.WithSignature(sig)
				if !flow.AddEndoresement(ed) {
					log.Error("failed to add endorsement")
				}
			}

			blk, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey)
			if err != nil {
				log.Error("flow.PackHeader", "error", err)
			}

			execElapsed := mclock.Now() - startTime

			// b := block.Compose(header, flow.Txs())
			err = s.commit(blk, stage, receipts)
			if err != nil {
				log.Error("s.pack", "error", err)
			}

			commitElapsed := mclock.Now() - execElapsed - startTime

			if !s.skipLogs {
				if err := s.logDB.Log(func(w *logdb.Writer) error {
					return w.Write(blk, receipts)
				}); err != nil {
					panic(errors.WithMessage(err, "commit log"))
				}
			}

			display(blk, receipts, prepareElapsed, execElapsed, commitElapsed)
		}
	}
}

func display(b *block.Block, receipts tx.Receipts, prepareElapsed, execElapsed, commitElapsed mclock.AbsTime) {
	blockID := b.Header().ID()
	log.Info("📦 new vip193 block packed",
		"txs", len(receipts),
		"mgas", float64(b.Header().GasUsed())/1000/1000,
		"et", fmt.Sprintf("%v|%v|%v", common.PrettyDuration(prepareElapsed), common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
		"id", fmt.Sprintf("[#%v…%x]", block.Number(blockID), blockID[28:]),
	)
	log.Debug(b.String())
}

func (s *Solo) commit(b *block.Block, stage *state.Stage, receipts tx.Receipts) error {
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

	return nil
}

// func (s *Solo) endorse(done chan struct{}, edCh chan *block.Endorsement, bs *block.Summary) {
// 	for i := uint64(0); i < thor.CommitteeSize*2; i++ {
// 		_, sk := vrf.GenKeyPair()
// 		ok, proof, err := s.cons.IsCommittee(sk, bs.Timestamp())
// 		if err != nil {
// 			panic(err)
// 		}
// 		if ok {
// 			ed := block.NewEndorsement(bs, proof)
// 			sig, _ := crypto.Sign(ed.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
// 			ed = ed.WithSignature(sig)

// 			select {
// 			case <-done:
// 				return
// 			case edCh <- ed:
// 				return
// 			}
// 		}
// 	}
// }

func (s *Solo) packTxSetAndBlockSummary(flow *packer.Flow, maxTxPackingDur int) error {
	var txsToRemove []*tx.Transaction
	defer func() {
		for _, tx := range txsToRemove {
			s.txPool.Remove(tx.Hash(), tx.ID())
		}
	}()

	done := make(chan struct{})
	go func() {
		time.Sleep(time.Duration(maxTxPackingDur) * time.Second)
		done <- struct{}{}
	}()

	for _, tx := range s.txPool.Executables() {
		select {
		case <-done:
			break
		default:
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

	_, _, err := flow.PackTxSetAndBlockSummary(genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		return err
	}

	return nil
}

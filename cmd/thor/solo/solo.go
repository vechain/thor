// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"context"
	"fmt"
	"math/big"
	"math/rand/v2"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/bandwidth"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

var (
	logger       = log.WithContext("pkg", "solo")
	baseGasPrice = big.NewInt(1e13)
)

type Options struct {
	GasLimit         uint64
	SkipLogs         bool
	MinTxPriorityFee uint64
	OnDemand         bool
	BlockInterval    uint64
}

// Solo mode is the standalone client without p2p server
type Solo struct {
	repo      *chain.Repository
	stater    *state.Stater
	txPool    *txpool.TxPool
	packer    *packer.Packer
	logDB     *logdb.LogDB
	bandwidth bandwidth.Bandwidth
	options   Options
}

// New returns Solo instance
func New(
	repo *chain.Repository,
	stater *state.Stater,
	logDB *logdb.LogDB,
	txPool *txpool.TxPool,
	forkConfig *thor.ForkConfig,
	options Options,
) *Solo {
	return &Solo{
		repo:   repo,
		stater: stater,
		txPool: txPool,
		packer: packer.New(
			repo,
			stater,
			genesis.DevAccounts()[0].Address,
			&genesis.DevAccounts()[0].Address,
			forkConfig,
			options.MinTxPriorityFee,
		),
		logDB:   logDB,
		options: options,
	}
}

// Run runs the packer for solo
func (s *Solo) Run(ctx context.Context) error {
	goes := &co.Goes{}

	defer func() {
		<-ctx.Done()
		goes.Wait()
	}()

	logger.Info("prepared to pack block")

	if err := s.init(ctx); err != nil {
		return err
	}

	goes.Go(func() {
		s.loop(ctx)
	})

	return nil
}

func (s *Solo) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Info("stopping interval packing service......")
			return
		case <-time.After(time.Duration(1) * time.Second):
			if left := uint64(time.Now().Unix()) % s.options.BlockInterval; left == 0 {
				if err := s.packing(s.txPool.Executables(), false); err != nil {
					logger.Error("failed to pack block", "err", err)
				}
			} else if s.options.OnDemand {
				pendingTxs := s.txPool.Executables()
				if len(pendingTxs) > 0 {
					if err := s.packing(pendingTxs, true); err != nil {
						logger.Error("failed to pack block", "err", err)
					}
				}
			}
		}
	}
}

func (s *Solo) packing(pendingTxs tx.Transactions, onDemand bool) error {
	best := s.repo.BestBlockSummary()
	now := uint64(time.Now().Unix())

	var txsToRemove []*tx.Transaction
	defer func() {
		for _, tx := range txsToRemove {
			s.txPool.Remove(tx.Hash(), tx.ID())
		}
	}()

	if s.options.GasLimit == 0 {
		suggested := s.bandwidth.SuggestGasLimit()
		s.packer.SetTargetGasLimit(suggested)
	}

	flow, _, err := s.packer.Mock(best, now, s.options.GasLimit)
	if err != nil {
		return errors.WithMessage(err, "mock packer")
	}

	startTime := mclock.Now()
	for _, tx := range pendingTxs {
		if err := flow.Adopt(tx); err != nil {
			if packer.IsGasLimitReached(err) {
				break
			}
			if packer.IsTxNotAdoptableNow(err) {
				continue
			}
			txsToRemove = append(txsToRemove, tx)
		}
	}

	b, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey, 0, false)
	if err != nil {
		return errors.WithMessage(err, "pack")
	}
	execElapsed := mclock.Now() - startTime

	// If there is no tx packed in the on-demanded block then skip
	if onDemand && len(b.Transactions()) == 0 {
		return nil
	}

	if _, err := stage.Commit(); err != nil {
		return errors.WithMessage(err, "commit state")
	}

	if !s.options.SkipLogs {
		w := s.logDB.NewWriter()
		if err := w.Write(b, receipts); err != nil {
			return errors.WithMessage(err, "write logs")
		}

		if err := w.Commit(); err != nil {
			return errors.WithMessage(err, "commit logs")
		}
	}

	// ignore fork when solo
	if err := s.repo.AddBlock(b, receipts, 0, true); err != nil {
		return errors.WithMessage(err, "commit block")
	}
	realElapsed := mclock.Now() - startTime

	commitElapsed := mclock.Now() - startTime - execElapsed

	if v, updated := s.bandwidth.Update(b.Header(), time.Duration(realElapsed)); updated {
		logger.Debug("bandwidth updated", "gps", v)
	}

	blockID := b.Header().ID()
	logger.Info("📦 new block packed",
		"txs", len(receipts),
		"mgas", float64(b.Header().GasUsed())/1000/1000,
		"et", fmt.Sprintf("%v|%v", common.PrettyDuration(execElapsed), common.PrettyDuration(commitElapsed)),
		"id", fmt.Sprintf("[#%v…%x]", block.Number(blockID), blockID[28:]),
	)
	logger.Debug(b.String())

	return nil
}

// The init function initializes the chain parameters.
func (s *Solo) init(ctx context.Context) error {
	best := s.repo.BestBlockSummary()
	newState := s.stater.NewState(best.Root())
	currentBGP, err := builtin.Params.Native(newState).Get(thor.KeyLegacyTxBaseGasPrice)
	if err != nil {
		return errors.WithMessage(err, "failed to get the current base gas price")
	}
	if currentBGP == baseGasPrice {
		return nil
	}

	method, found := builtin.Params.ABI.MethodByName("set")
	if !found {
		return errors.New("Params ABI: set method not found")
	}

	data, err := method.EncodeInput(thor.KeyLegacyTxBaseGasPrice, baseGasPrice)
	if err != nil {
		return err
	}

	clause := tx.NewClause(&builtin.Params.Address).WithData(data)
	baseGasPriceTx, err := s.newTx([]*tx.Clause{clause}, genesis.DevAccounts()[0])
	if err != nil {
		return err
	}

	if !s.options.OnDemand {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(int64(s.options.BlockInterval)-time.Now().Unix()%int64(s.options.BlockInterval)) * time.Second):
		}
	}

	return s.packing(tx.Transactions{baseGasPriceTx}, false)
}

// newTx builds and signs a new transaction from the given clauses
func (s *Solo) newTx(clauses []*tx.Clause, from genesis.DevAccount) (*tx.Transaction, error) {
	builder := new(tx.Builder).ChainTag(s.repo.ChainTag())
	for _, c := range clauses {
		builder.Clause(c)
	}

	trx := builder.BlockRef(tx.NewBlockRef(0)).
		Expiration(math.MaxUint32).
		Nonce(rand.Uint64()). //#nosec G404
		DependsOn(nil).
		Gas(1_000_000).
		Build()

	return tx.Sign(trx, from.PrivateKey)
}

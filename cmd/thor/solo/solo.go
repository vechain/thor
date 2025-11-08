// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"context"
	"math/big"
	"math/rand/v2"
	"time"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/log"
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
	repo    *chain.Repository
	stater  *state.Stater
	txPool  TxPool
	options Options
	core    *Core // Core is used to pack blocks
}

type TxPool interface {
	txpool.Pool
	// Executables returns the transactions that can be executed
	Executables() tx.Transactions
	// Remove removes a transaction from the pool
	Remove(txHash thor.Bytes32, txID thor.Bytes32) bool
}

// New returns Solo instance
func New(
	repo *chain.Repository,
	stater *state.Stater,
	txPool TxPool,
	options Options,
	core *Core,
) *Solo {
	return &Solo{
		repo:    repo,
		stater:  stater,
		txPool:  txPool,
		options: options,
		core:    core,
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
				if txs, err := s.core.Pack(s.txPool.Executables(), false); err != nil {
					logger.Error("failed to pack block", "err", err)
				} else {
					for _, tx := range txs {
						s.txPool.Remove(tx.Hash(), tx.ID())
					}
				}
			}
		}
	}
}

// The init function initializes the chain parameters.
func (s *Solo) init(ctx context.Context) error {
	best := s.repo.BestBlockSummary()
	newState := s.stater.NewState(best.Root())
	currentBGP, err := builtin.Params.Native(newState).Get(thor.KeyLegacyTxBaseGasPrice)
	if err != nil {
		return errors.WithMessage(err, "failed to get the current base gas price")
	}
	if currentBGP != nil && currentBGP.Cmp(baseGasPrice) == 0 {
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
	if _, err := s.core.Pack(tx.Transactions{baseGasPriceTx}, false); err != nil {
		return errors.WithMessage(err, "failed to pack base gas price transaction")
	}

	return nil
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

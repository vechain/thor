// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"context"
	"math/big"
	"time"

	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
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
	engine  *Engine // Engine is used to pack blocks
}

type TxPool interface {
	transactions.Pool
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
	engine *Engine,
) *Solo {
	return &Solo{
		repo:    repo,
		stater:  stater,
		txPool:  txPool,
		options: options,
		engine:  engine,
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
				if txs, err := s.engine.Pack(s.txPool.Executables(), false); err != nil {
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

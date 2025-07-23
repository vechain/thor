// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/bandwidth"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

type Engine struct {
	repo       *chain.Repository
	stater     *state.Stater
	packer     *packer.Packer
	logDB      *logdb.LogDB
	bandwidth  bandwidth.Bandwidth
	options    Options
	forkConfig *thor.ForkConfig
	mu         sync.Mutex // protects the Pack method
}

func NewEngine(
	repo *chain.Repository,
	stater *state.Stater,
	logDB *logdb.LogDB,
	options Options,
	forkConfig *thor.ForkConfig,
) *Engine {
	return &Engine{
		repo:   repo,
		stater: stater,
		packer: packer.New(
			repo,
			stater,
			genesis.DevAccounts()[0].Address,
			&genesis.DevAccounts()[0].Address,
			forkConfig,
			options.MinTxPriorityFee,
		),
		logDB:      logDB,
		options:    options,
		forkConfig: forkConfig,
	}
}

func (e *Engine) Pack(pendingTxs tx.Transactions, onDemand bool) ([]*tx.Transaction, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	best := e.repo.BestBlockSummary()
	now := uint64(time.Now().Unix())

	var txsToRemove []*tx.Transaction

	if e.options.GasLimit == 0 {
		suggested := e.bandwidth.SuggestGasLimit()
		e.packer.SetTargetGasLimit(suggested)
	}

	flow, _, err := e.packer.Mock(best, now, e.options.GasLimit)
	if err != nil {
		return nil, errors.WithMessage(err, "mock packer")
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
		return nil, errors.WithMessage(err, "pack")
	}
	execElapsed := mclock.Now() - startTime

	// If there is no tx packed in the on-demanded block then skip
	if onDemand && len(b.Transactions()) == 0 {
		return nil, nil
	}

	if _, err := stage.Commit(); err != nil {
		return nil, errors.WithMessage(err, "commit state")
	}

	if !e.options.SkipLogs {
		w := e.logDB.NewWriter()
		if err := w.Write(b, receipts); err != nil {
			return nil, errors.WithMessage(err, "write logs")
		}

		if err := w.Commit(); err != nil {
			return nil, errors.WithMessage(err, "commit logs")
		}
	}

	// ignore fork when solo
	if err := e.repo.AddBlock(b, receipts, 0, true); err != nil {
		return nil, errors.WithMessage(err, "commit block")
	}
	realElapsed := mclock.Now() - startTime
	commitElapsed := mclock.Now() - startTime - execElapsed

	if v, updated := e.bandwidth.Update(b.Header(), time.Duration(realElapsed)); updated {
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

	return txsToRemove, nil
}

func (e *Engine) IsExecutable(trx *tx.Transaction) (bool, error) {
	best := e.repo.BestBlockSummary()
	chain := e.repo.NewChain(best.Header.ID())
	state := e.stater.NewState(best.Root())

	baseFee := galactica.CalcBaseFee(best.Header, e.forkConfig)

	txObject, err := txpool.ResolveTx(trx, true)
	if err != nil {
		return false, errors.WithMessage(err, "resolve transaction")
	}

	return txObject.Executable(chain, state, best.Header, e.forkConfig, baseFee)
}

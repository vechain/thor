// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"context"
	"errors"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var errEthPoolNotImplemented = errors.New("eth pool: not implemented")

// EthPool maintains unprocessed Ethereum-family transactions.
// This is a scaffold: admission, nonce promotion, wash, and housekeeping
// will be implemented in follow-up changes.
//
// When admission lands, place/promote/drop/wash paths must reserve/release on
// the shared costTracker exactly like VeChainPool — never call
// into VeChainPool directly.
type EthPool struct {
	options      Options
	repo         *chain.Repository
	stater       *state.Stater
	forkConfig   *thor.ForkConfig
	costs        *costTracker
	baseFeeCache *baseFeeCache

	all *ethPoolMap

	ctx    context.Context
	cancel func()
	txFeed event.Feed
	scope  event.SubscriptionScope
	goes   sync.WaitGroup
}

var _ Pool = (*EthPool)(nil)

// NewEth creates a new EthPool stub with its own cost tracker.
// Close must be called at shutdown. Prefer NewCoordinator when both family
// pools must share one ledger.
func NewEth(repo *chain.Repository, stater *state.Stater, options Options, forkConfig *thor.ForkConfig) *EthPool {
	return newEthPool(repo, stater, options, forkConfig, newCostTracker())
}

// newEthPool creates an EthPool. costs is required (dependency injection).
func newEthPool(
	repo *chain.Repository,
	stater *state.Stater,
	options Options,
	forkConfig *thor.ForkConfig,
	costs *costTracker,
) *EthPool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &EthPool{
		options:      options,
		repo:         repo,
		stater:       stater,
		forkConfig:   forkConfig,
		costs:        costs,
		baseFeeCache: newBaseFeeCache(forkConfig),
		all:          newEthPoolMap(costs),
		ctx:          ctx,
		cancel:       cancel,
	}
	pool.goes.Go(func() {
		<-pool.ctx.Done()
	})
	return pool
}

func (p *EthPool) Get(txID thor.Bytes32) *tx.Transaction {
	return p.GetByHash(txID)
}

func (p *EthPool) GetByHash(hash thor.Bytes32) *tx.Transaction {
	if txObj := p.all.GetByHash(hash); txObj != nil {
		return txObj.Transaction
	}
	return nil
}

func (p *EthPool) AddRemote(newTx *tx.Transaction) error {
	if newTx == nil || !newTx.IsEthereumTx() {
		return badTxError{"invalid tx type for Ethereum pool"}
	}
	if p.all.GetByHash(newTx.Hash()) != nil {
		return txRejectedError{errEthAlreadyKnown.Error()}
	}
	if err := validateTxBasics(p.repo, p.forkConfig, newTx); err != nil {
		return err
	}
	txObj, err := ResolveTx(newTx, false)
	if err != nil {
		return badTxError{err.Error()}
	}

	head := p.repo.BestBlockSummary()
	if newTx.Gas() > head.Header.GasLimit() {
		return txRejectedError{"tx gas exceeds block gas limit"}
	}
	chainView := p.repo.NewChain(head.Header.ID())
	if known, err := chainView.HasTransaction(newTx.ID(), 0); err != nil {
		return err
	} else if known {
		return txRejectedError{"known tx"}
	}
	st := p.stater.NewState(head.Root())
	stateNonce, err := st.GetNonce(txObj.Origin())
	if err != nil {
		return err
	}
	if newTx.Nonce() < stateNonce {
		return txRejectedError{errEthNonceTooLow.Error()}
	}

	baseFee := p.baseFeeCache.Get(head.Header)
	prepare := func(obj *TxObject) (reservationRequest, bool, error) {
		if baseFee != nil && obj.MaxFeePerGas().Cmp(baseFee) < 0 {
			return reservationRequest{}, false, nil
		}
		checkpoint := st.NewCheckpoint()
		legacyBase, _, payer, prepaid, _, err := obj.resolved.BuyGas(
			st,
			head.Header.Timestamp()+thor.BlockInterval(),
			baseFee,
		)
		st.RevertTo(checkpoint)
		if err != nil {
			return reservationRequest{}, false, err
		}
		normalizedBaseFee := baseFee
		if normalizedBaseFee == nil {
			normalizedBaseFee = new(big.Int)
		}
		obj.payer = &payer
		obj.cost = prepaid
		obj.priorityGasPrice = obj.EffectivePriorityFeePerGas(normalizedBaseFee, legacyBase, nil)
		balance, err := builtin.Energy.Native(
			st,
			head.Header.Timestamp()+thor.BlockInterval(),
		).Get(payer)
		if err != nil {
			return reservationRequest{}, false, err
		}
		return reservationRequest{
			owner:   ethReservationOwner(obj.Origin(), obj.Nonce()),
			payer:   payer,
			cost:    prepaid,
			balance: balance,
		}, true, nil
	}

	executable, promoted, err := p.all.add(
		txObj,
		stateNonce,
		p.options.Limit,
		p.options.EthAccountSlots,
		p.options.EthAccountQueue,
		p.options.EthPriceBump,
		prepare,
	)
	if err != nil {
		return txRejectedError{err.Error()}
	}
	p.goes.Go(func() {
		p.txFeed.Send(&TxEvent{Tx: newTx, Executable: &executable})
		promotedExecutable := true
		for _, promotedTx := range promoted {
			p.txFeed.Send(&TxEvent{Tx: promotedTx.Transaction, Executable: &promotedExecutable})
		}
	})
	logger.Trace("Ethereum tx added", "id", newTx.ID(), "executable", executable)
	return nil
}

func (p *EthPool) ReinjectFromFork(newTx *tx.Transaction) error {
	_ = newTx
	return errEthPoolNotImplemented
}

func (p *EthPool) AddLocal(newTx *tx.Transaction) error {
	_ = newTx
	return errEthPoolNotImplemented
}

func (p *EthPool) StrictlyAdd(newTx *tx.Transaction) error {
	_ = newTx
	return errEthPoolNotImplemented
}

func (p *EthPool) Remove(txHash thor.Bytes32, txID thor.Bytes32) bool {
	_, _ = txHash, txID
	return false
}

func (p *EthPool) Dump() tx.Transactions {
	return nil
}

func (p *EthPool) Len() int {
	return p.all.Len()
}

func (p *EthPool) SubscribeTxEvent(ch chan *TxEvent) event.Subscription {
	return p.scope.Track(p.txFeed.Subscribe(ch))
}

func (p *EthPool) Executables() tx.Transactions {
	return nil
}

func (p *EthPool) Fill(txs tx.Transactions) {
	_ = txs
}

func (p *EthPool) PoolNonce(addr thor.Address) uint64 {
	if nonce, ok := p.all.poolNonceOK(addr); ok {
		return nonce
	}
	head := p.repo.BestBlockSummary()
	nonce, err := p.stater.NewState(head.Root()).GetNonce(addr)
	if err != nil {
		return 0
	}
	return nonce
}

func (p *EthPool) Close() {
	p.all.pruneEmptySenders()
	p.cancel()
	p.scope.Close()
	p.goes.Wait()
}

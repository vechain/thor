// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"context"
	"errors"
	"sync"

	"github.com/ethereum/go-ethereum/event"

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
	options    Options
	repo       *chain.Repository
	stater     *state.Stater
	forkConfig *thor.ForkConfig
	costs      *costTracker

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
		options:    options,
		repo:       repo,
		stater:     stater,
		forkConfig: forkConfig,
		costs:      costs,
		all:        newEthPoolMap(),
		ctx:        ctx,
		cancel:     cancel,
	}
	pool.goes.Go(func() {
		<-pool.ctx.Done()
	})
	return pool
}

func (p *EthPool) Get(txID thor.Bytes32) *tx.Transaction {
	// Scaffold: ID-based lookup will use a dedicated index once admission lands.
	_ = txID
	return nil
}

func (p *EthPool) GetByHash(hash thor.Bytes32) *tx.Transaction {
	if txObj := p.all.GetByHash(hash); txObj != nil {
		return txObj.Transaction
	}
	return nil
}

func (p *EthPool) AddRemote(newTx *tx.Transaction) error {
	_ = newTx
	return errEthPoolNotImplemented
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
	return p.all.poolNonce(addr)
}

func (p *EthPool) Close() {
	p.all.pruneEmptySenders()
	p.cancel()
	p.scope.Close()
	p.goes.Wait()
}

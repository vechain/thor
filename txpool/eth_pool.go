// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"math/big"
	"slices"
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

type ethAdmissionContext struct {
	head        *chain.BlockSummary
	state       *state.State
	stateNonces map[thor.Address]uint64
	prepare     ethPrepare
}

func (p *EthPool) newAdmissionContext() *ethAdmissionContext {
	head := p.repo.BestBlockSummary()
	st := p.stater.NewState(head.Root())
	baseFee := p.baseFeeCache.Get(head.Header)
	ctx := &ethAdmissionContext{
		head:        head,
		state:       st,
		stateNonces: make(map[thor.Address]uint64),
	}
	ctx.prepare = func(obj *TxObject) (reservationRequest, bool, error) {
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
	return ctx
}

func (ctx *ethAdmissionContext) stateNonce(origin thor.Address) (uint64, error) {
	if nonce, cached := ctx.stateNonces[origin]; cached {
		return nonce, nil
	}
	nonce, err := ctx.state.GetNonce(origin)
	if err != nil {
		return 0, err
	}
	ctx.stateNonces[origin] = nonce
	return nonce, nil
}

func (p *EthPool) resolveAdmission(
	newTx *tx.Transaction,
	ctx *ethAdmissionContext,
	duplicateNoop bool,
) (*TxObject, uint64, bool, error) {
	if newTx == nil || !newTx.IsEthereumTx() {
		return nil, 0, false, badTxError{"invalid tx type for Ethereum pool"}
	}
	if p.all.GetByHash(newTx.Hash()) != nil {
		if duplicateNoop {
			return nil, 0, true, nil
		}
		return nil, 0, false, txRejectedError{errEthAlreadyKnown.Error()}
	}
	if err := validateTxBasics(p.repo, p.forkConfig, newTx); err != nil {
		return nil, 0, false, err
	}
	txObj, err := ResolveTx(newTx, false)
	if err != nil {
		return nil, 0, false, badTxError{err.Error()}
	}
	if newTx.Gas() > ctx.head.Header.GasLimit() {
		return nil, 0, false, txRejectedError{"tx gas exceeds block gas limit"}
	}
	chainView := p.repo.NewChain(ctx.head.Header.ID())
	if known, err := chainView.HasTransaction(newTx.ID(), 0); err != nil {
		return nil, 0, false, err
	} else if known {
		return nil, 0, false, txRejectedError{"known tx"}
	}
	stateNonce, err := ctx.stateNonce(txObj.Origin())
	if err != nil {
		return nil, 0, false, err
	}
	if newTx.Nonce() < stateNonce {
		return nil, 0, false, txRejectedError{errEthNonceTooLow.Error()}
	}
	return txObj, stateNonce, false, nil
}

func (p *EthPool) emitAdmission(newTx *tx.Transaction, executable bool, promoted []*TxObject) {
	p.goes.Go(func() {
		p.txFeed.Send(&TxEvent{Tx: newTx, Executable: &executable})
		promotedExecutable := true
		for _, promotedTx := range promoted {
			p.txFeed.Send(&TxEvent{Tx: promotedTx.Transaction, Executable: &promotedExecutable})
		}
	})
}

func (p *EthPool) AddRemote(newTx *tx.Transaction) error {
	ctx := p.newAdmissionContext()
	txObj, stateNonce, _, err := p.resolveAdmission(newTx, ctx, false)
	if err != nil {
		return err
	}

	executable, promoted, err := p.all.add(
		txObj,
		stateNonce,
		p.options.Limit,
		p.options.EthAccountSlots,
		p.options.EthAccountQueue,
		p.options.EthPriceBump,
		ctx.prepare,
	)
	if err != nil {
		return txRejectedError{err.Error()}
	}
	p.emitAdmission(newTx, executable, promoted)
	logger.Trace("Ethereum tx added", "id", newTx.ID(), "executable", executable)
	return nil
}

func (p *EthPool) ReinjectFromFork(fork ForkReinjection) error {
	ctx := p.newAdmissionContext()
	if err := p.collectIncludedForkNonces(ctx, fork.Included); err != nil {
		return err
	}

	candidates, err := p.collectForkCandidates(ctx, fork.Discarded)
	if err != nil {
		return err
	}
	sortEthForkCandidates(candidates)

	results, err := p.all.reconcileFork(
		candidates,
		ctx.stateNonces,
		p.options.Limit,
		p.options.EthAccountSlots,
		p.options.EthAccountQueue,
		p.options.EthPriceBump,
		ctx.prepare,
	)
	if err != nil {
		return err
	}
	p.emitForkResults(results)
	return nil
}

func (p *EthPool) collectIncludedForkNonces(
	ctx *ethAdmissionContext,
	included tx.Transactions,
) error {
	for _, includedTx := range included {
		if includedTx == nil || !includedTx.IsEthereumTx() {
			continue
		}
		origin, err := includedTx.Origin()
		if err != nil {
			return err
		}
		if _, err := ctx.stateNonce(origin); err != nil {
			return err
		}
	}
	return nil
}

func (p *EthPool) collectForkCandidates(
	ctx *ethAdmissionContext,
	discarded tx.Transactions,
) ([]ethForkCandidate, error) {
	candidates := make([]ethForkCandidate, 0, len(discarded))
	for _, discardedTx := range discarded {
		if discardedTx == nil || !discardedTx.IsEthereumTx() {
			continue
		}
		if origin, err := discardedTx.Origin(); err == nil {
			if _, err := ctx.stateNonce(origin); err != nil {
				return nil, err
			}
		}

		txObj, stateNonce, duplicate, err := p.resolveAdmission(discardedTx, ctx, true)
		if err != nil {
			if IsBadTx(err) || IsTxRejected(err) {
				logger.Debug("failed to reinject Ethereum tx", "err", err, "id", discardedTx.ID())
				continue
			}
			return nil, err
		}
		if !duplicate {
			candidates = append(candidates, ethForkCandidate{txObj: txObj, stateNonce: stateNonce})
		}
	}
	return candidates, nil
}

func sortEthForkCandidates(candidates []ethForkCandidate) {
	slices.SortStableFunc(candidates, func(a, b ethForkCandidate) int {
		aOrigin, bOrigin := a.txObj.Origin(), b.txObj.Origin()
		if addressCmp := bytes.Compare(aOrigin[:], bOrigin[:]); addressCmp != 0 {
			return addressCmp
		}
		return cmp.Compare(a.txObj.Nonce(), b.txObj.Nonce())
	})
}

func (p *EthPool) emitForkResults(results []ethForkResult) {
	for _, result := range results {
		if result.err != nil {
			logger.Debug("failed to reinject Ethereum tx", "err", result.err, "id", result.txObj.ID())
			continue
		}
		p.emitAdmission(result.txObj.Transaction, result.executable, result.promoted)
	}
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

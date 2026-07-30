// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var errEthPoolNotImplemented = errors.New("eth pool: not implemented")

// EthPool is the Ethereum-family Pool facade. It owns chain admission,
// housekeeping, and event publication; ethPoolCore owns transaction state,
// mutation ordering, and its locks.
//
// TODO: complete strict admission, partitioned Fill, the VeChain
// wrong-family guard, and family-aware metrics.
// Every future mutation path must use the shared costTracker and must never
// call into VeChainPool directly.
// AddLocal and Fill are intentionally no-ops until local/stash admission is wired.
type EthPool struct {
	options      Options
	repo         *chain.Repository
	stater       *state.Stater
	forkConfig   *thor.ForkConfig
	baseFeeCache *baseFeeCache
	blocklist    *blocklist

	core *ethPoolCore

	ctx    context.Context
	cancel func()
	txFeed event.Feed
	scope  event.SubscriptionScope
	goes   sync.WaitGroup
}

var _ Pool = (*EthPool)(nil)

// NewEth creates a new EthPool stub with its own cost tracker.
// Close must be called at shutdown. Prefer NewCoordinator when both family
// pools must share one ledger
func newEthPool(
	repo *chain.Repository,
	stater *state.Stater,
	options Options,
	forkConfig *thor.ForkConfig,
	costTracker *costTracker,
	blocked *blocklist,
) *EthPool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &EthPool{
		options:      options,
		repo:         repo,
		stater:       stater,
		forkConfig:   forkConfig,
		baseFeeCache: newBaseFeeCache(forkConfig),
		blocklist:    blocked,
		core:         newEthPoolCore(costTracker, options),
		ctx:          ctx,
		cancel:       cancel,
	}
	pool.goes.Go(pool.housekeeping)
	pool.goes.Go(pool.fetchBlocklistLoop)
	return pool
}

// TODO: do we need to expose an api for the cost tracker?

func (p *EthPool) fetchBlocklistLoop() {
	runBlocklistLoop(p.ctx, p.options, p.blocklist)
}

func (p *EthPool) housekeeping() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	revalidatedHead := p.repo.BestBlockSummary().Header.ID()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			nextHead, err := p.runHousekeepingTick(revalidatedHead)
			if err != nil {
				logger.Warn("failed to maintain Ethereum pool", "err", err)
				continue
			}
			revalidatedHead = nextHead
		}
	}
}

// runHousekeepingTick sweeps every tick and revalidates only when the head has
// moved since the last successful revalidate. A transaction admitted at head H was
// already validated against H, so nothing about its viability can change until the
// head changes; only expiry and capacity can, and sweeping covers those without
// touching chain state.
func (p *EthPool) runHousekeepingTick(revalidatedHead thor.Bytes32) (thor.Bytes32, error) {
	if p.core.Len() == 0 {
		return revalidatedHead, nil
	}
	if err := p.sweep(); err != nil {
		return revalidatedHead, err
	}

	currentHead := p.repo.BestBlockSummary()
	if currentHead.Header.ID() == revalidatedHead {
		return revalidatedHead, nil
	}
	if !isChainSynced(uint64(time.Now().Unix()), currentHead.Header.Timestamp()) {
		return revalidatedHead, nil
	}
	if err := p.revalidate(currentHead); err != nil {
		return revalidatedHead, err
	}
	return currentHead.Header.ID(), nil
}

func (p *EthPool) sweep() error {
	result, err := p.core.sweep()
	if err != nil {
		return err
	}
	p.emitExecutableChanges(result.demoted, false)
	logger.Trace("Ethereum pool sweep complete", "removed", result.removed)
	return nil
}

// revalidate is the safety net behind the block-commit reconcile path: it re-reads
// every sender's canonical nonce and affordability at the given head. Reconcile
// keeps the pool correct on its own, so this exists to recover from a reconcile
// that failed or was never delivered.
// TODO: should we remove this?
func (p *EthPool) revalidate(head *chain.BlockSummary) error {
	ctx := p.newAdmissionContextAt(head)
	for _, origin := range p.core.origins() {
		if _, err := ctx.stateNonce(origin); err != nil {
			return err
		}
	}
	promoted, demoted, err := p.core.revalidate(
		ctx.stateNonces,
		p.options.EthAccountSlots,
		ctx.prepare,
	)
	if err != nil {
		return err
	}
	p.emitExecutableChanges(demoted, false)
	for _, txObj := range promoted {
		p.emitAdmission(txObj.Transaction, true, nil)
	}
	logger.Trace("Ethereum pool revalidate complete", "promoted", len(promoted))
	return nil
}

func (p *EthPool) Get(txID thor.Bytes32) *tx.Transaction {
	return p.GetByHash(txID)
}

func (p *EthPool) GetByHash(hash thor.Bytes32) *tx.Transaction {
	if txObj := p.core.GetByHash(hash); txObj != nil {
		return txObj.Transaction
	}
	return nil
}

// emitAdmission notifies subscribers about every executable-status change caused
// by the newTx admission.
func (p *EthPool) emitAdmission(newTx *tx.Transaction, executable bool, promoted []*TxObject) {
	p.goes.Go(func() {
		p.txFeed.Send(&TxEvent{Tx: newTx, Executable: &executable})
		promotedExecutable := true
		for _, promotedTx := range promoted {
			p.txFeed.Send(&TxEvent{Tx: promotedTx.Transaction, Executable: &promotedExecutable})
		}
	})
}

func (p *EthPool) emitExecutableChanges(txObjs []*TxObject, executable bool) {
	if len(txObjs) == 0 {
		return
	}
	p.goes.Go(func() {
		for _, txObj := range txObjs {
			status := executable
			p.txFeed.Send(&TxEvent{Tx: txObj.Transaction, Executable: &status})
		}
	})
}

func (p *EthPool) AddRemote(newTx *tx.Transaction) error {
	txObj, stateNonce, skip, ctx, err := p.resolveRemoteAdmission(newTx)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	executable, promoted, demoted, err := p.core.addWithTransitions(
		txObj,
		stateNonce,
		p.options.Limit,
		p.options.EthAccountSlots,
		p.options.EthAccountQueue,
		p.options.EthPriceBump,
		ctx.prepare,
	)
	if err != nil {
		p.emitExecutableChanges(demoted, false)
		return txRejectedError{err.Error()}
	}
	p.emitAdmission(newTx, executable, promoted)
	p.emitExecutableChanges(demoted, false)
	logger.Trace("Ethereum tx added", "id", newTx.ID(), "executable", executable)
	return nil
}

// ReconcileOnHeadChange reconciles the pool with a newly canonical head. The node calls
// it synchronously on every block commit, which makes it the single owner of nonce
// reconciliation; the housekeeping revalidate is only a safety net behind it.
func (p *EthPool) ReconcileOnHeadChange(headChange HeadChangeTxs) error {
	if len(headChange.Discarded) == 0 {
		// Forward extension, the common case by far: no transaction returned to the
		// pool, so nonces only moved forward and the affected senders can be synced
		// in place instead of torn down and rebuilt.
		return p.syncIncludedOrigins(headChange.Included)
	}
	return p.reconcileReorg(headChange)
}

// syncIncludedOrigins advances the senders whose transactions just became
// canonical, and promotes whatever that frees.
func (p *EthPool) syncIncludedOrigins(included tx.Transactions) error {
	ctx := p.newAdmissionContext()
	if err := p.collectIncludedForkNonces(ctx, included); err != nil {
		return err
	}
	if len(ctx.stateNonces) == 0 {
		return nil
	}

	promoted, demoted, err := p.core.syncHeadWithTransitions(
		ctx.stateNonces,
		p.options.EthAccountSlots,
		ctx.prepare,
	)
	if err != nil {
		return err
	}
	p.emitExecutableChanges(demoted, false)
	for _, txObj := range promoted {
		p.emitAdmission(txObj.Transaction, true, nil)
	}
	return nil
}

// reconcileReorg handles a real reorg: transactions are coming back to the pool, so
// every affected sender is reset to its canonical nonce and rebuilt.
func (p *EthPool) reconcileReorg(headChange HeadChangeTxs) error {
	ctx := p.newAdmissionContext()
	if err := p.collectIncludedForkNonces(ctx, headChange.Included); err != nil {
		return err
	}

	candidates, err := p.collectForkCandidates(ctx, headChange.Discarded)
	if err != nil {
		return err
	}
	sortEthForkCandidates(candidates)

	results, demoted, err := p.core.reconcileForkWithTransitions(
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
	p.emitExecutableChanges(demoted, false)
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

		txObj, stateNonce, skip, err := p.resolveReinjectAdmission(discardedTx, ctx)
		if err != nil {
			if IsBadTx(err) || IsTxRejected(err) {
				logger.Debug("failed to reinject Ethereum tx", "err", err, "id", discardedTx.ID())
				continue
			}
			return nil, err
		}
		if !skip {
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

// AddLocal admits a locally submitted Ethereum tx (e.g. REST POST /transactions).
// Local privilege policy for Eth is not differentiated yet, so admission matches
// AddRemote.
// TODO: temorarily just delegate to AddRemote, but should be refactor to implement local privilege policy.
func (p *EthPool) AddLocal(newTx *tx.Transaction) error {
	return p.AddRemote(newTx)
}

func (p *EthPool) StrictlyAdd(newTx *tx.Transaction) error {
	_ = newTx
	return errEthPoolNotImplemented
}

func (p *EthPool) Remove(txHash thor.Bytes32, txID thor.Bytes32) bool {
	txObj := p.core.GetByHash(txHash)
	if txObj == nil || txObj.ID() != txID {
		return false
	}
	removed, demoted := p.core.removeByHashWithTransitions(txHash)
	if !removed {
		return false
	}
	p.emitExecutableChanges(demoted, false)
	logger.Debug("Ethereum tx removed", "id", txID)
	return true
}

func (p *EthPool) Dump() tx.Transactions {
	return p.core.ToTxs()
}

func (p *EthPool) Len() int {
	return p.core.Len()
}

func (p *EthPool) SubscribeTxEvent(ch chan *TxEvent) event.Subscription {
	return p.scope.Track(p.txFeed.Subscribe(ch))
}

func (p *EthPool) Executables() tx.Transactions {
	return p.executableSnapshot().transactions()
}

// executableSnapshot exposes immutable merge metadata without revealing the
// core's storage or lock ownership to the coordinator.
func (p *EthPool) executableSnapshot() ethExecutablesSnapshot {
	return p.core.executableSnapshot()
}

func (p *EthPool) Fill(txs tx.Transactions) {
	_ = txs
}

func (p *EthPool) PoolNonce(addr thor.Address) uint64 {
	if nonce, ok := p.core.poolNonceOK(addr); ok {
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
	p.cancel()
	p.scope.Close()
	p.goes.Wait()
	p.core.pruneEmptySenders()
}

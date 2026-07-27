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
	"sync/atomic"
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
	costs        *costTracker
	baseFeeCache *baseFeeCache
	blocklist    *blocklist

	core *ethPoolCore

	addedAfterWash atomic.Uint32
	ctx            context.Context
	cancel         func()
	txFeed         event.Feed
	scope          event.SubscriptionScope
	goes           sync.WaitGroup
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
	return newEthPoolWithBlocklist(repo, stater, options, forkConfig, costs, new(blocklist), true)
}

func newEthPoolWithBlocklist(
	repo *chain.Repository,
	stater *state.Stater,
	options Options,
	forkConfig *thor.ForkConfig,
	costs *costTracker,
	blocked *blocklist,
	manageBlocklist bool,
) *EthPool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &EthPool{
		options:      options,
		repo:         repo,
		stater:       stater,
		forkConfig:   forkConfig,
		costs:        costs,
		baseFeeCache: newBaseFeeCache(forkConfig),
		blocklist:    blocked,
		core:         newEthPoolCore(costs),
		ctx:          ctx,
		cancel:       cancel,
	}
	pool.goes.Go(pool.housekeeping)
	if manageBlocklist {
		pool.goes.Go(func() {
			runBlocklistLoop(pool.ctx, pool.options, pool.blocklist)
		})
	}
	return pool
}

func (p *EthPool) housekeeping() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	processedHead := p.repo.BestBlockSummary()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			nextHead, err := p.runHousekeepingTick(processedHead)
			if err != nil {
				logger.Warn("failed to maintain Ethereum pool", "err", err)
				continue
			}
			processedHead = nextHead
		}
	}
}

func (p *EthPool) runHousekeepingTick(processedHead *chain.BlockSummary) (*chain.BlockSummary, error) {
	currentHead := p.repo.BestBlockSummary()
	headChanged := currentHead.Header.ID() != processedHead.Header.ID()
	poolLen := p.core.Len()
	overLimit := p.options.Limit > 0 && poolLen > p.options.Limit
	if !headChanged && !overLimit && p.addedAfterWash.Load() <= 0 {
		return processedHead, nil
	}
	if !isChainSynced(uint64(time.Now().Unix()), currentHead.Header.Timestamp()) {
		return processedHead, nil
	}
	if headChanged {
		if err := p.processHeadChange(processedHead, currentHead); err != nil {
			return processedHead, err
		}
	}
	if err := p.wash(currentHead); err != nil {
		return processedHead, err
	}
	p.addedAfterWash.Store(0)
	if headChanged {
		return currentHead, nil
	}
	return processedHead, nil
}

func (p *EthPool) wash(head *chain.BlockSummary) error {
	ctx := p.newAdmissionContextAt(head)
	for _, origin := range p.core.origins() {
		if _, err := ctx.stateNonce(origin); err != nil {
			return err
		}
	}
	result, err := p.core.wash(
		ctx.stateNonces,
		ethWashOptions{
			now:          time.Now().UnixNano(),
			maxLifetime:  p.options.MaxLifetime,
			pendingLimit: p.options.EthAccountSlots,
			queueLimit:   p.options.EthAccountQueue,
			globalLimit:  p.options.Limit,
		},
		ctx.prepare,
	)
	if err != nil {
		return err
	}
	p.emitExecutableChanges(result.demoted, false)
	for _, txObj := range result.promoted {
		p.emitAdmission(txObj.Transaction, true, nil)
	}
	logger.Trace("Ethereum pool wash complete", "removed", result.removed, "promoted", len(result.promoted))
	return nil
}

func (p *EthPool) processHeadChange(previous, current *chain.BlockSummary) error {
	if current.Header.Number() <= previous.Header.Number() {
		// Reorg arms at the same or a lower height are reconciled synchronously
		// through ReinjectFromFork.
		return nil
	}

	origins := make(map[thor.Address]struct{})
	canonical := p.repo.NewChain(current.Header.ID())
	for number := previous.Header.Number() + 1; ; number++ {
		blk, err := canonical.GetBlock(number)
		if err != nil {
			return err
		}
		for _, trx := range blk.Transactions() {
			if !trx.IsEthereumTx() {
				continue
			}
			origin, err := trx.Origin()
			if err != nil {
				return err
			}
			origins[origin] = struct{}{}
		}
		if number == current.Header.Number() {
			break
		}
	}
	if len(origins) == 0 {
		return nil
	}

	ctx := p.newAdmissionContextAt(current)
	for origin := range origins {
		if _, err := ctx.stateNonce(origin); err != nil {
			return err
		}
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

func (p *EthPool) Get(txID thor.Bytes32) *tx.Transaction {
	return p.GetByHash(txID)
}

func (p *EthPool) GetByHash(hash thor.Bytes32) *tx.Transaction {
	if txObj := p.core.GetByHash(hash); txObj != nil {
		return txObj.Transaction
	}
	return nil
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
	ctx := p.newAdmissionContext()
	txObj, stateNonce, noop, err := p.resolveAdmission(newTx, ctx, false)
	if err != nil {
		return err
	}
	if noop {
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
	p.addedAfterWash.Add(1)
	p.emitAdmission(newTx, executable, promoted)
	p.emitExecutableChanges(demoted, false)
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

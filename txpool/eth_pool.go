// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Per-account limits for EthPool (PoC hard-coded; promote to Options when
// the split-pool wiring is productionized).
const (
	ethLimitPerAccountPending = 64
	ethLimitPerAccountQueue   = 64
)

// EthPool is the nonce-aware sub-pool dedicated to EthereumTx (TypeEthTyped1559).
// It maintains per-sender pending/queue buckets (see ethSender) and uses the
// Ethereum wire hash as the dedup / indexing key. VeChain-native txs are
// rejected at Add; routing is enforced by TxPool.
type EthPool struct {
	repo       *chain.Repository
	stater     *state.Stater
	forkConfig *thor.ForkConfig
	ethChainID uint64

	pool         *ethPoolMap
	baseFeeCache *baseFeeCache

	addedAfterWash uint32

	ctx    context.Context
	cancel func()
	txFeed event.Feed
	scope  event.SubscriptionScope
	goes   sync.WaitGroup
}

// NewEth creates a new EthPool.  Shutdown must be called via Close().
func NewEth(repo *chain.Repository, stater *state.Stater, forkConfig *thor.ForkConfig) *EthPool {
	ctx, cancel := context.WithCancel(context.Background())
	p := &EthPool{
		repo:         repo,
		stater:       stater,
		forkConfig:   forkConfig,
		ethChainID:   thor.GetEthChainID(repo.GenesisBlock().Header().ID()),
		pool:         newEthPoolMap(ethLimitPerAccountPending, ethLimitPerAccountQueue),
		baseFeeCache: newBaseFeeCache(forkConfig),
		ctx:          ctx,
		cancel:       cancel,
	}
	p.goes.Go(p.housekeeping)
	return p
}

// Close stops the housekeeping goroutine and detaches event subscribers.
func (p *EthPool) Close() {
	p.cancel()
	p.scope.Close()
	p.goes.Wait()
	logger.Debug("eth pool closed")
}

// SubscribeTxEvent is the EthPool equivalent of VeChainPool.SubscribeTxEvent.
// The coordinator subscribes to this feed and fans out onto its unified feed.
func (p *EthPool) SubscribeTxEvent(ch chan *TxEvent) event.Subscription {
	return p.scope.Track(p.txFeed.Subscribe(ch))
}

// Add accepts an EthereumTx into the pool.  Non-Eth txs are rejected.
func (p *EthPool) Add(newTx *tx.Transaction) error {
	return p.add(newTx, false, false)
}

// AddLocal behaves like Add but flags the tx as locally submitted.
func (p *EthPool) AddLocal(newTx *tx.Transaction) error {
	return p.add(newTx, false, true)
}

// StrictlyAdd requires the tx to be executable immediately (pending, not queued).
func (p *EthPool) StrictlyAdd(newTx *tx.Transaction) error {
	return p.add(newTx, true, false)
}

func (p *EthPool) add(newTx *tx.Transaction, rejectNonExecutable, localSubmitted bool) (err error) {
	source := "local"
	if !localSubmitted {
		source = "remote"
	}
	defer func() {
		if err != nil {
			metricBadTxGauge().AddWithLabel(1, map[string]string{"source": source})
		}
	}()

	if !newTx.IsEthereumTx() {
		return badTxError{"non-ethereum tx routed to EthPool"}
	}
	if err := p.validateTxBasics(newTx); err != nil {
		return err
	}
	if p.pool.containsHash(newTx.Hash()) {
		return nil
	}

	txObj, err := ResolveTx(newTx, localSubmitted)
	if err != nil {
		return badTxError{err.Error()}
	}

	headSummary := p.repo.BestBlockSummary()
	chainState := p.stater.NewState(headSummary.Root())
	baseFee := p.baseFeeCache.Get(headSummary.Header)
	executable, err := txObj.Executable(
		p.repo.NewChain(headSummary.Header.ID()),
		chainState,
		headSummary.Header,
		p.forkConfig,
		baseFee,
	)
	if err != nil {
		return txRejectedError{err.Error()}
	}
	// In EthPool, "executable" at Add-time is a necessary but not sufficient
	// signal — nonce alignment decides whether the tx is truly pending.
	txObj.executable = executable

	if rejectNonExecutable && !executable {
		return txRejectedError{"tx is not executable"}
	}

	// Read the canonical chain nonce so the sender entry is seeded correctly.
	chainNonce := p.chainNonce(txObj.Origin())
	replaced, err := p.pool.add(txObj, chainNonce)
	if err != nil {
		return txRejectedError{err.Error()}
	}

	atomic.AddUint32(&p.addedAfterWash, 1)
	metricTxPoolGauge().AddWithLabel(1, map[string]string{"source": source, "type": "EthTyped1559"})

	// Emit events: fire a removal signal for the replaced slot, then an add for
	// the new tx.  Executable flag is a pointer so nil means "not yet decided".
	if replaced != nil {
		metricTxPoolGauge().AddWithLabel(-1, map[string]string{"source": source, "type": "EthTyped1559"})
		rpt := replaced.Transaction
		p.goes.Go(func() {
			notExec := false
			p.txFeed.Send(&TxEvent{rpt, &notExec})
		})
	}
	p.goes.Go(func() {
		p.txFeed.Send(&TxEvent{newTx, &executable})
	})

	logger.Trace("eth tx added", "id", newTx.ID(), "executable", executable)
	return nil
}

func (p *EthPool) validateTxBasics(trx *tx.Transaction) error {
	if err := trx.EnforceSignatureLowS(); err != nil {
		return badTxError{err.Error()}
	}
	if trx.Size() > MaxTxSize {
		return txRejectedError{"size too large"}
	}

	nextBlockNum := p.repo.BestBlockSummary().Header.Number() + 1
	if !thor.IsForked(nextBlockNum, p.forkConfig.INTERSTELLAR) {
		return badTxError{"Ethereum EIP-1559 transactions are not supported before the INTERSTELLAR fork"}
	}
	if trx.Gas() > thor.MaxTxGasLimit {
		return badTxError{"tx gas limit exceeds the maximum allowed"}
	}
	if trx.EthChainID() != p.ethChainID {
		return badTxError{fmt.Sprintf("Ethereum chain ID %d does not match network chain ID %d",
			trx.EthChainID(), p.ethChainID)}
	}
	return nil
}

// Get returns the tx for a given Thor tx ID. For EthereumTx the ID is
// defined as the EIP-2718 wire hash, so this is equivalent to GetByHash;
// both methods exist on the Pool interface because the ID vs Hash
// distinction is meaningful for the VeChain family.
func (p *EthPool) Get(id thor.Bytes32) *tx.Transaction {
	if txObj := p.pool.getByHash(id); txObj != nil {
		return txObj.Transaction
	}
	return nil
}

// GetByHash looks up a pending EthereumTx by its EIP-2718 wire hash.
// For Ethereum-family txs the wire hash equals the Thor ID, so this
// returns the same value as Get(hash); the separate method exists to
// satisfy the Pool interface uniformly across families.
func (p *EthPool) GetByHash(hash thor.Bytes32) *tx.Transaction {
	if txObj := p.pool.getByHash(hash); txObj != nil {
		return txObj.Transaction
	}
	return nil
}

// PoolNonce returns the next expected nonce for addr, accounting for in-pool
// pending txs.  Used by eth_getTransactionCount("pending").
func (p *EthPool) PoolNonce(addr thor.Address) uint64 {
	return p.pool.poolNonce(addr, p.chainNonce(addr))
}

// chainNonce reads the persisted Ethereum account nonce from the canonical
// chain state at the current best block. Returns 0 if the address has never
// sent an EthereumTx.
func (p *EthPool) chainNonce(addr thor.Address) uint64 {
	headSummary := p.repo.BestBlockSummary()
	st := p.stater.NewState(headSummary.Root())
	n, err := st.GetNonce(addr)
	if err != nil {
		logger.Warn("eth pool: failed to read chain nonce", "addr", addr, "err", err)
		return 0
	}
	return n
}

// Remove drops a tx by its hash/id.
func (p *EthPool) Remove(txHash thor.Bytes32, _ thor.Bytes32) bool {
	if p.pool.removeByHash(txHash) {
		metricTxPoolGauge().AddWithLabel(-1, map[string]string{"source": "n/a", "type": "EthTyped1559"})
		return true
	}
	return false
}

// Len returns the total number of Ethereum txs held (pending + queued).
func (p *EthPool) Len() int {
	return p.pool.len()
}

// Dump returns a flat snapshot of every Ethereum tx in the pool.
func (p *EthPool) Dump() tx.Transactions {
	all, _ := p.pool.snapshot()
	out := make(tx.Transactions, 0, len(all))
	for _, t := range all {
		out = append(out, t.Transaction)
	}
	return out
}

// Executables returns pending EthereumTx grouped per sender in ascending nonce
// order, then concatenated.  Within a sender, nonce N precedes N+1 (the
// invariant the coordinator's merge must preserve globally).  Absolute ordering
// across senders is EFS-unaware here — the coordinator does the global EFS
// merge across families via executablePendingGroups().
func (p *EthPool) Executables() tx.Transactions {
	_, groups := p.pool.snapshot()
	var total int
	for _, g := range groups {
		total += len(g)
	}
	out := make(tx.Transactions, 0, total)
	for _, g := range groups {
		for _, t := range g {
			out = append(out, t.Transaction)
		}
	}
	return out
}

// executablePendingGroups is the coordinator-facing snapshot: one sub-slice per
// sender, each in ascending nonce order.  Lives in the same package so the
// coordinator can access it without exporting an interface.
//
// Each returned TxObject has its priorityGasPrice set to the EIP-1559 effective
// priority fee: min(maxPriorityFeePerGas, maxFeePerGas − baseFee).  This is
// computed here (rather than relying on Executable or revalidateAgainstHead)
// so the coordinator's merge heap always sees a current, non-nil value.
func (p *EthPool) executablePendingGroups() [][]*TxObject {
	_, groups := p.pool.snapshot()
	baseFee := p.baseFeeCache.Get(p.repo.BestBlockSummary().Header)
	if baseFee == nil {
		baseFee = new(big.Int)
	}
	for _, g := range groups {
		for _, txObj := range g {
			txObj.priorityGasPrice = txObj.EffectivePriorityFeePerGas(baseFee, nil, nil)
		}
	}
	return groups
}

// Fill ingests pre-resolved transactions (used for pool state handoff during
// restart).  Skips per-account quotas and cost checks, matching VeChainPool.Fill.
func (p *EthPool) Fill(txs tx.Transactions) {
	for _, t := range txs {
		if !t.IsEthereumTx() {
			continue
		}
		txObj, err := ResolveTx(t, false)
		if err != nil {
			continue
		}
		_, _ = p.pool.add(txObj, p.chainNonce(txObj.Origin()))
	}
}

// housekeeping runs a 1s ticker mirroring VeChainPool's; on head-change it
// walks the delta and bumps stateNonce for every observed EthereumTx
// inclusion.  It also re-evaluates pending/queued txs against the new head
// (affordability, expiry, etc.) and evicts anything no longer viable.
func (p *EthPool) housekeeping() {
	logger.Debug("enter eth housekeeping")
	defer logger.Debug("leave eth housekeeping")

	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	head := p.repo.BestBlockSummary()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			newHead := p.repo.BestBlockSummary()
			if newHead.Header.ID() == head.Header.ID() {
				continue
			}
			p.processHeadChange(head, newHead)
			head = newHead
		}
	}
}

// processHeadChange advances stateNonces and re-checks pending txs after a
// head update.  prevHead..newHead is walked linearly; a production impl would
// handle reorgs by using chain.NewChain(newHead).  PoC assumes linear growth.
func (p *EthPool) processHeadChange(prevHead, newHead *chain.BlockSummary) {
	chainView := p.repo.NewChain(newHead.Header.ID())

	// Walk prev.Number+1 .. new.Number inclusive.
	for n := prevHead.Header.Number() + 1; n <= newHead.Header.Number(); n++ {
		blk, err := chainView.GetBlock(n)
		if err != nil {
			logger.Warn("eth pool: failed to read block during head walk", "num", n, "err", err)
			break
		}
		for _, includedTx := range blk.Transactions() {
			if !includedTx.IsEthereumTx() {
				continue
			}
			origin, err := includedTx.Origin()
			if err != nil {
				continue
			}
			// The chain's next-expected nonce for this sender is now at least
			// the included tx's nonce + 1.
			p.pool.bumpStateNonce(origin, includedTx.Nonce()+1)
		}
	}

	// Re-check executable state for everything that survived the bumps.
	p.revalidateAgainstHead(newHead)
	atomic.StoreUint32(&p.addedAfterWash, 0)
}

// revalidateAgainstHead walks every pooled tx once and removes any that is no
// longer Executable against the new head.  This is a simpler analog of
// VeChainPool.wash focused on the semantics EthereumTx cares about (affordability,
// expiry, max gas).
func (p *EthPool) revalidateAgainstHead(head *chain.BlockSummary) {
	all, _ := p.pool.snapshot()
	baseFee := p.baseFeeCache.Get(head.Header)
	chainView := p.repo.NewChain(head.Header.ID())

	for _, txObj := range all {
		s := p.stater.NewState(head.Root())
		// Reset the cached executable flag so Executable re-computes cost/payer.
		txObj.executable = false
		exec, err := txObj.Executable(chainView, s, head.Header, p.forkConfig, baseFee)
		if err != nil || !exec {
			p.pool.removeByHash(txObj.Hash())
			logger.Trace("eth tx washed out", "id", txObj.ID(), "err", err)
			continue
		}
		txObj.executable = true
	}
}

// compile-time assertion that EthPool implements Pool.
var _ Pool = (*EthPool)(nil)

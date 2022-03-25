// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"context"
	"math/rand"
	"os"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/event"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const (
	// max size of tx allowed
	maxTxSize = 64 * 1024
)

var (
	log = log15.New("pkg", "txpool")
)

// Options options for tx pool.
type Options struct {
	Limit                  int
	LimitPerAccount        int
	MaxLifetime            time.Duration
	BlocklistCacheFilePath string
	BlocklistFetchURL      string
}

// TxEvent will be posted when tx is added or status changed.
type TxEvent struct {
	Tx         *tx.Transaction
	Executable *bool
}

// TxPool maintains unprocessed transactions.
type TxPool struct {
	options   Options
	repo      *chain.Repository
	stater    *state.Stater
	blocklist blocklist

	executables    atomic.Value
	all            *txObjectMap
	addedAfterWash uint32

	ctx    context.Context
	cancel func()
	txFeed event.Feed
	scope  event.SubscriptionScope
	goes   co.Goes
}

// New create a new TxPool instance.
// Shutdown is required to be called at end.
func New(repo *chain.Repository, stater *state.Stater, options Options) *TxPool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &TxPool{
		options: options,
		repo:    repo,
		stater:  stater,
		all:     newTxObjectMap(),
		ctx:     ctx,
		cancel:  cancel,
	}

	pool.goes.Go(pool.housekeeping)
	pool.goes.Go(pool.fetchBlocklistLoop)
	return pool
}

func (p *TxPool) housekeeping() {
	log.Debug("enter housekeeping")
	defer log.Debug("leave housekeeping")

	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	headSummary := p.repo.BestBlockSummary()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			var headBlockChanged bool
			if newHeadSummary := p.repo.BestBlockSummary(); newHeadSummary.Header.ID() != headSummary.Header.ID() {
				headSummary = newHeadSummary
				headBlockChanged = true
			}
			if !isChainSynced(uint64(time.Now().Unix()), headSummary.Header.Timestamp()) {
				// skip washing txs if not synced
				continue
			}
			poolLen := p.all.Len()
			// do wash on
			// 1. head block changed
			// 2. pool size exceeds limit
			// 3. new tx added while pool size is small
			if headBlockChanged ||
				poolLen > p.options.Limit ||
				(poolLen < 200 && atomic.LoadUint32(&p.addedAfterWash) > 0) {

				atomic.StoreUint32(&p.addedAfterWash, 0)

				startTime := mclock.Now()
				executables, removed, err := p.wash(headSummary)
				elapsed := mclock.Now() - startTime

				ctx := []interface{}{
					"len", poolLen,
					"removed", removed,
					"elapsed", common.PrettyDuration(elapsed),
				}
				if err != nil {
					ctx = append(ctx, "err", err)
				} else {
					p.executables.Store(executables)
				}

				log.Debug("wash done", ctx...)
			}
		}
	}
}

func (p *TxPool) fetchBlocklistLoop() {
	var (
		path = p.options.BlocklistCacheFilePath
		url  = p.options.BlocklistFetchURL
	)

	if path != "" {
		if err := p.blocklist.Load(path); err != nil {
			if !os.IsNotExist(err) {
				log.Warn("blocklist load failed", "error", err, "path", path)
			}
		} else {
			log.Debug("blocklist loaded", "len", p.blocklist.Len())
		}
	}
	if url == "" {
		return
	}

	var eTag string
	fetch := func() {
		if err := p.blocklist.Fetch(p.ctx, url, &eTag); err != nil {
			if err == context.Canceled {
				return
			}
			log.Warn("blocklist fetch failed", "error", err, "url", url)
		} else {
			log.Debug("blocklist fetched", "len", p.blocklist.Len())
			if path != "" {
				if err := p.blocklist.Save(path); err != nil {
					log.Warn("blocklist save failed", "error", err, "path", path)
				} else {
					log.Debug("blocklist saved")
				}
			}
		}
	}

	fetch()

	for {
		// delay 1~2 min
		delay := time.Second * time.Duration(rand.Int()%60+60)
		select {
		case <-p.ctx.Done():
			return
		case <-time.After(delay):
			fetch()
		}
	}
}

// Close cleanup inner go routines.
func (p *TxPool) Close() {
	p.cancel()
	p.scope.Close()
	p.goes.Wait()
	log.Debug("closed")
}

//SubscribeTxEvent receivers will receive a tx
func (p *TxPool) SubscribeTxEvent(ch chan *TxEvent) event.Subscription {
	return p.scope.Track(p.txFeed.Subscribe(ch))
}

func (p *TxPool) add(newTx *tx.Transaction, rejectNonexecutable bool, localSubmitted bool) error {
	if p.all.ContainsHash(newTx.Hash()) {
		// tx already in the pool
		return nil
	}

	origin, _ := newTx.Origin()
	if thor.IsOriginBlocked(origin) || p.blocklist.Contains(origin) {
		// tx origin blocked
		return nil
	}

	headSummary := p.repo.BestBlockSummary()

	// validation
	switch {
	case newTx.ChainTag() != p.repo.ChainTag():
		return badTxError{"chain tag mismatch"}
	case newTx.Size() > maxTxSize:
		return txRejectedError{"size too large"}
	}

	if err := newTx.TestFeatures(headSummary.Header.TxsFeatures()); err != nil {
		return txRejectedError{err.Error()}
	}

	txObj, err := resolveTx(newTx, localSubmitted)
	if err != nil {
		return badTxError{err.Error()}
	}

	if isChainSynced(uint64(time.Now().Unix()), headSummary.Header.Timestamp()) {
		if !localSubmitted {
			// reject when pool size exceeds 120% of limit
			if p.all.Len() >= p.options.Limit*12/10 {
				return txRejectedError{"pool is full"}
			}
		}

		state := p.stater.NewState(headSummary.Header.StateRoot(), headSummary.Header.Number(), headSummary.Conflicts, headSummary.SteadyNum)
		executable, err := txObj.Executable(p.repo.NewChain(headSummary.Header.ID()), state, headSummary.Header)
		if err != nil {
			return txRejectedError{err.Error()}
		}

		if rejectNonexecutable && !executable {
			return txRejectedError{"tx is not executable"}
		}

		if err := p.all.Add(txObj, p.options.LimitPerAccount); err != nil {
			return txRejectedError{err.Error()}
		}

		txObj.executable = executable
		p.goes.Go(func() {
			p.txFeed.Send(&TxEvent{newTx, &executable})
		})
		log.Debug("tx added", "id", newTx.ID(), "executable", executable)
	} else {
		// we skip steps that rely on head block when chain is not synced,
		// but check the pool's limit
		if p.all.Len() >= p.options.Limit {
			return txRejectedError{"pool is full"}
		}

		if err := p.all.Add(txObj, p.options.LimitPerAccount); err != nil {
			return txRejectedError{err.Error()}
		}
		log.Debug("tx added", "id", newTx.ID())
		p.goes.Go(func() {
			p.txFeed.Send(&TxEvent{newTx, nil})
		})
	}
	atomic.AddUint32(&p.addedAfterWash, 1)
	return nil
}

// Add add new tx into pool.
// It's not assumed as an error if the tx to be added is already in the pool,
func (p *TxPool) Add(newTx *tx.Transaction) error {
	return p.add(newTx, false, false)
}

// AddLocal adds new locally submitted tx into pool.
func (p *TxPool) AddLocal(newTx *tx.Transaction) error {
	return p.add(newTx, false, true)
}

// Get get pooled tx by id.
func (p *TxPool) Get(id thor.Bytes32) *tx.Transaction {
	if txObj := p.all.GetByID(id); txObj != nil {
		return txObj.Transaction
	}
	return nil
}

// StrictlyAdd add new tx into pool. A rejection error will be returned, if tx is not executable at this time.
func (p *TxPool) StrictlyAdd(newTx *tx.Transaction) error {
	return p.add(newTx, true, false)
}

// Remove removes tx from pool by its Hash.
func (p *TxPool) Remove(txHash thor.Bytes32, txID thor.Bytes32) bool {
	if p.all.RemoveByHash(txHash) {
		log.Debug("tx removed", "id", txID)
		return true
	}
	return false
}

// Executables returns executable txs.
func (p *TxPool) Executables() tx.Transactions {
	if sorted := p.executables.Load(); sorted != nil {
		return sorted.(tx.Transactions)
	}
	return nil
}

// Fill fills txs into pool.
func (p *TxPool) Fill(txs tx.Transactions, localSubmitted bool) {
	txObjs := make([]*txObject, 0, len(txs))
	for _, tx := range txs {
		origin, _ := tx.Origin()
		if thor.IsOriginBlocked(origin) || p.blocklist.Contains(origin) {
			continue
		}
		// here we ignore errors
		if txObj, err := resolveTx(tx, localSubmitted); err == nil {
			txObjs = append(txObjs, txObj)
		}
	}
	p.all.Fill(txObjs)
}

// Dump dumps all txs in the pool.
func (p *TxPool) Dump() tx.Transactions {
	return p.all.ToTxs()
}

// wash to evict txs that are over limit, out of lifetime, out of energy, settled, expired or dep broken.
// this method should only be called in housekeeping go routine
func (p *TxPool) wash(headSummary *chain.BlockSummary) (executables tx.Transactions, removed int, err error) {
	all := p.all.ToTxObjects()
	var toRemove []*txObject
	defer func() {
		if err != nil {
			// in case of error, simply cut pool size to limit
			for i, txObj := range all {
				if len(all)-i <= p.options.Limit {
					break
				}
				removed++
				p.all.RemoveByHash(txObj.Hash())
			}
		} else {
			for _, txObj := range toRemove {
				p.all.RemoveByHash(txObj.Hash())
			}
			removed = len(toRemove)
		}
	}()

	// recreate state everytime to avoid high RAM usage when the pool at hight water-mark.
	newState := func() *state.State {
		return p.stater.NewState(headSummary.Header.StateRoot(), headSummary.Header.Number(), headSummary.Conflicts, headSummary.SteadyNum)
	}
	baseGasPrice, err := builtin.Params.Native(newState()).Get(thor.KeyBaseGasPrice)
	if err != nil {
		return nil, 0, err
	}

	var (
		chain               = p.repo.NewChain(headSummary.Header.ID())
		executableObjs      = make([]*txObject, 0, len(all))
		nonExecutableObjs   = make([]*txObject, 0, len(all))
		localExecutableObjs = make([]*txObject, 0, len(all))
		now                 = time.Now().UnixNano()
	)
	for _, txObj := range all {
		if thor.IsOriginBlocked(txObj.Origin()) || p.blocklist.Contains(txObj.Origin()) {
			toRemove = append(toRemove, txObj)
			log.Debug("tx washed out", "id", txObj.ID(), "err", "blocked")
			continue
		}

		// out of lifetime
		if !txObj.localSubmitted && now > txObj.timeAdded+int64(p.options.MaxLifetime) {
			toRemove = append(toRemove, txObj)
			log.Debug("tx washed out", "id", txObj.ID(), "err", "out of lifetime")
			continue
		}
		// settled, out of energy or dep broken
		executable, err := txObj.Executable(chain, newState(), headSummary.Header)
		if err != nil {
			toRemove = append(toRemove, txObj)
			log.Debug("tx washed out", "id", txObj.ID(), "err", err)
			continue
		}

		if executable {
			provedWork, err := txObj.ProvedWork(headSummary.Header.Number(), chain.GetBlockID)
			if err != nil {
				toRemove = append(toRemove, txObj)
				log.Debug("tx washed out", "id", txObj.ID(), "err", err)
				continue
			}
			txObj.overallGasPrice = txObj.OverallGasPrice(baseGasPrice, provedWork)
			if txObj.localSubmitted {
				localExecutableObjs = append(localExecutableObjs, txObj)
			} else {
				executableObjs = append(executableObjs, txObj)
			}
		} else {
			if !txObj.localSubmitted {
				nonExecutableObjs = append(nonExecutableObjs, txObj)
			}
		}
	}

	// sort objs by price from high to low.
	sortTxObjsByOverallGasPriceDesc(executableObjs)

	limit := p.options.Limit

	// remove over limit txs, from non-executables to low priced
	if len(executableObjs) > limit {
		for _, txObj := range nonExecutableObjs {
			toRemove = append(toRemove, txObj)
			log.Debug("non-executable tx washed out due to pool limit", "id", txObj.ID())
		}
		for _, txObj := range executableObjs[limit:] {
			toRemove = append(toRemove, txObj)
			log.Debug("executable tx washed out due to pool limit", "id", txObj.ID())
		}
		executableObjs = executableObjs[:limit]
	} else if len(executableObjs)+len(nonExecutableObjs) > limit {
		// executableObjs + nonExecutableObjs over pool limit
		for _, txObj := range nonExecutableObjs[limit-len(executableObjs):] {
			toRemove = append(toRemove, txObj)
			log.Debug("non-executable tx washed out due to pool limit", "id", txObj.ID())
		}
	}

	// Concate executables.
	executableObjs = append(executableObjs, localExecutableObjs...)
	// Sort will be faster (part of it already sorted).
	sortTxObjsByOverallGasPriceDesc(executableObjs)

	executables = make(tx.Transactions, 0, len(executableObjs))
	var toBroadcast tx.Transactions

	for _, obj := range executableObjs {
		executables = append(executables, obj.Transaction)
		if !obj.executable || obj.localSubmitted {
			obj.executable = true
			toBroadcast = append(toBroadcast, obj.Transaction)
		}
	}

	p.goes.Go(func() {
		for _, tx := range toBroadcast {
			executable := true
			p.txFeed.Send(&TxEvent{tx, &executable})
		}
	})
	return executables, 0, nil
}

func isChainSynced(nowTimestamp, blockTimestamp uint64) bool {
	timeDiff := nowTimestamp - blockTimestamp
	if blockTimestamp > nowTimestamp {
		timeDiff = blockTimestamp - nowTimestamp
	}
	return timeDiff < thor.BlockInterval*6
}

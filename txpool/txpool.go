package txpool

import (
	"errors"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//PoolConfig PoolConfig
type PoolConfig struct {
	PoolSize int           // Maximum number of executable transaction slots for all accounts
	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}

//DefaultTxPoolConfig DefaultTxPoolConfig
var defaultTxPoolConfig = PoolConfig{
	PoolSize: 20000,
	Lifetime: 1000,
}

type objectStatus int

const (
	Pending objectStatus = iota
	Queued
)

//txObject wrap transaction
type txObject struct {
	tx           *tx.Transaction
	status       objectStatus
	overallGP    *big.Int
	creationTime int64
}

//TxPool TxPool
type TxPool struct {
	config PoolConfig
	chain  *chain.Chain
	stateC *state.Creator
	goes   co.Goes
	done   chan struct{}
	all    *cache.RandCache
	rw     sync.RWMutex
	txFeed event.Feed
	scope  event.SubscriptionScope
}

//New construct a new txpool
func New(chain *chain.Chain, stateC *state.Creator) *TxPool {
	pool := &TxPool{
		config: defaultTxPoolConfig,
		chain:  chain,
		stateC: stateC,
		done:   make(chan struct{}),
	}
	pool.all = cache.NewRandCache(pool.config.PoolSize)
	pool.goes.Go(pool.dequeue)
	return pool
}

//Add transaction
func (pool *TxPool) Add(txs ...*tx.Transaction) error {
	pool.rw.Lock()
	defer pool.rw.Unlock()

	var err error
	for _, tx := range txs {
		if err = pool.add(tx); err != nil {
			return err
		}
	}

	return nil
}

func (pool *TxPool) add(tx *tx.Transaction) error {
	txID := tx.ID()
	if _, ok := pool.all.Get(txID); ok {
		return errors.New("known transaction")
	}

	// If the transaction fails basic validation, discard it
	if err := pool.validateTx(tx); err != nil {
		return err
	}

	if pool.all.Len() > pool.config.PoolSize {
		if picked, ok := pool.all.Pick().Value.(*txObject); ok {
			pool.all.Remove(picked.tx.ID())
		}
	}

	bestBlock, err := pool.chain.GetBestBlock()
	if err != nil {
		return err
	}

	sp, err := pool.shouldPending(tx, bestBlock)
	if err != nil {
		return err
	}

	to := &txObject{
		tx:           tx,
		overallGP:    new(big.Int),
		creationTime: time.Now().Unix(),
	}
	if sp {
		to.status = Pending
	} else {
		to.status = Queued
	}
	pool.all.Set(txID, to)

	pool.goes.Go(func() { pool.txFeed.Send(tx) })

	return nil
}

//SubscribeNewTransaction receivers will receive a tx
func (pool *TxPool) SubscribeNewTransaction(ch chan *tx.Transaction) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

func (pool *TxPool) shouldPending(tx *tx.Transaction, bestBlock *block.Block) (bool, error) {
	dependsOn := tx.DependsOn()
	if dependsOn != nil {
		if _, _, err := pool.chain.GetTransaction(*dependsOn); err != nil {
			if pool.chain.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
	}
	blockNum := tx.BlockRef().Number()
	if blockNum > bestBlock.Header().Number() {
		return false, nil
	}
	return true, nil
}

//Dump dump transactions by TransactionCategory
func (pool *TxPool) Dump() []*tx.Transaction {
	bestBlock, err := pool.chain.GetBestBlock()
	if err != nil {
		return nil
	}
	pendingObjs := pool.pendingObjs(bestBlock, false)
	txs := make([]*tx.Transaction, len(pendingObjs))
	for i, obj := range pendingObjs {
		txs[i] = obj.tx
	}
	return txs
}

//Pending return all pending txs
func (pool *TxPool) Pending() []*tx.Transaction {
	bestBlock, err := pool.chain.GetBestBlock()
	if err != nil {
		return nil
	}
	pendingObjs := pool.pendingObjs(bestBlock, true)
	txs := make([]*tx.Transaction, len(pendingObjs))
	for i, obj := range pendingObjs {
		txs[i] = obj.tx
	}
	return txs
}

func (pool *TxPool) pendingObjs(bestBlock *block.Block, shouldSort bool) []*txObject {
	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return nil
	}
	baseGasPrice := builtin.Params.Native(st).Get(thor.KeyBaseGasPrice)
	traverser := pool.chain.NewTraverser(bestBlock.Header().ID())
	all := pool.allObjs()
	var pendings []*txObject
	for id, obj := range all {
		if obj.tx.IsExpired(bestBlock.Header().Number()) || time.Now().Unix()-obj.creationTime > int64(pool.config.Lifetime) {
			pool.Remove(id)
			continue
		}
		if obj.status == Pending {
			overallGP := obj.tx.OverallGasPrice(baseGasPrice, bestBlock.Header().Number(), func(num uint32) thor.Bytes32 {
				return traverser.Get(num).ID()
			})
			obj.overallGP = overallGP
			pendings = append(pendings, obj)
		}
	}
	if shouldSort {
		sort.Slice(pendings, func(i, j int) bool {
			return pendings[i].overallGP.Cmp(pendings[j].overallGP) > 0
		})
	}
	return pendings
}

//Remove remove transaction by txID with TransactionCategory
func (pool *TxPool) Remove(txIDs ...thor.Bytes32) {
	pool.rw.Lock()
	defer pool.rw.Unlock()

	for _, txID := range txIDs {
		pool.all.Remove(txID)
	}
}

//dequeueTxs for dequeue transactions
func (pool *TxPool) dequeue() {
	ticker := time.NewTicker(1 * time.Second)
	var bestBlock *block.Block
	defer ticker.Stop()
	for {
		select {
		case <-pool.done:
			return
		case <-ticker.C:
			b, err := pool.chain.GetBestBlock()
			if err != nil {
				continue
			}
			if bestBlock == nil {
				bestBlock = b
			} else {
				if b.Header().ID() == bestBlock.Header().ID() {
					continue
				}
				bestBlock = b
			}
			pool.update(bestBlock)
		}
	}
}

func (pool *TxPool) update(bestBlock *block.Block) {
	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return
	}

	baseGasPrice := builtin.Params.Native(st).Get(thor.KeyBaseGasPrice)
	traverser := pool.chain.NewTraverser(bestBlock.Header().ID())

	all := pool.allObjs()
	//can be pendinged txObjects
	for id, obj := range all {
		if obj.tx.IsExpired(bestBlock.Header().Number()) || time.Now().Unix()-obj.creationTime > int64(pool.config.Lifetime) {
			pool.Remove(id)
			continue
		}
		if obj.status == Queued {
			sp, err := pool.shouldPending(obj.tx, bestBlock)
			if err != nil {
				return
			}
			if sp {
				overallGP := obj.tx.OverallGasPrice(baseGasPrice, bestBlock.Header().Number(), func(num uint32) thor.Bytes32 {
					return traverser.Get(num).ID()
				})
				obj.overallGP = overallGP
				obj.status = Pending
				pool.all.Set(id, obj)
			}
		}
	}
}

func (pool *TxPool) allObjs() map[thor.Bytes32]*txObject {
	pool.rw.RLock()
	defer pool.rw.RUnlock()
	all := make(map[thor.Bytes32]*txObject)
	pool.all.ForEach(func(entry *cache.Entry) bool {
		if obj, ok := entry.Value.(*txObject); ok {
			if key, ok := entry.Key.(thor.Bytes32); ok {
				all[key] = obj
				return true
			}
		}
		return false
	})
	return all
}

//Shutdown shutdown pool loop
func (pool *TxPool) Shutdown() {
	close(pool.done)
	pool.scope.Close()
	pool.goes.Wait()
}

//GetTransaction returns a transaction
func (pool *TxPool) GetTransaction(txID thor.Bytes32) *tx.Transaction {
	pool.rw.RLock()
	defer pool.rw.RUnlock()
	if res, ok := pool.all.Get(txID); ok {
		if obj, ok := res.(*txObject); ok {
			return obj.tx
		}
	}
	return nil
}

func (pool *TxPool) validateTx(tx *tx.Transaction) error {
	if tx.Size() > 32*1024 {
		return errors.New("tx too large")
	}

	bestBlock, err := pool.chain.GetBestBlock()
	if err != nil {
		return err
	}

	st, err := pool.stateC.NewState(bestBlock.Header().StateRoot())
	if err != nil {
		return err
	}

	resolvedTx, err := runtime.ResolveTransaction(st, tx)
	if err != nil {
		return err
	}

	_, _, err = resolvedTx.BuyGas(bestBlock.Header().Number() + 1)
	if err != nil {
		return err
	}

	for _, clause := range resolvedTx.Clauses {
		if clause.Value().Sign() < 0 {
			return errors.New("negative clause value")
		}
	}

	return nil
}

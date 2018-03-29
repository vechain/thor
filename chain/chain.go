package chain

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain/persist"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const (
	blockCacheLimit        = 512
	blockTxIDsLimit        = 1024
	receiptsCacheLimit     = 512
	trunkBlockIDCacheLimit = 2048
)

var errNotFound = errors.New("not found")
var errBlockExist = errors.New("block already exists")

// Chain describes a persistent block chain.
// It's thread-safe.
type Chain struct {
	kv        kv.GetPutter
	bestBlock atomic.Value
	caches    caches
	rw        sync.RWMutex
}

type caches struct {
	block        *cache
	txIDs        *cache
	receipts     *cache
	trunkBlockID *cache
}

// New create an instance of Chain.
func New(kv kv.GetPutter) *Chain {

	blockCache := newLRU(blockCacheLimit, func(key interface{}) (interface{}, error) {
		raw, err := persist.LoadRawBlock(kv, key.(thor.Hash))
		if err != nil {
			return nil, err
		}
		return &rawBlock{raw: raw}, nil
	})
	txIDsCache := newLRU(blockTxIDsLimit, func(key interface{}) (interface{}, error) {
		block, err := blockCache.GetOrLoad(key)
		if err != nil {
			return nil, err
		}
		ids := make(map[thor.Hash]int)
		body, err := block.(*rawBlock).Body()
		if err != nil {
			return nil, err
		}
		for i, tx := range body.Txs {
			ids[tx.ID()] = i
		}
		return ids, nil
	})

	receiptsCache := newLRU(receiptsCacheLimit, func(key interface{}) (interface{}, error) {
		return persist.LoadBlockReceipts(kv, key.(thor.Hash))
	})

	trunkBlockIDCache := newLRU(trunkBlockIDCacheLimit, func(key interface{}) (interface{}, error) {
		return persist.LoadTrunkBlockID(kv, key.(uint32))
	})

	return &Chain{
		kv: kv,
		caches: caches{
			block:        blockCache,
			txIDs:        txIDsCache,
			receipts:     receiptsCache,
			trunkBlockID: trunkBlockIDCache,
		},
	}
}

// WriteGenesis writes in genesis block.
// It will compare the given genesis block with the existed one. If not the same, an error returned.
func (c *Chain) WriteGenesis(genesis *block.Block) error {
	c.rw.Lock()
	defer c.rw.Unlock()

	b0, err := c.getBlockByNumber(0)
	if err != nil {
		if !c.IsNotFound(err) {
			// errors occurred
			return err
		}
		// no genesis yet
		batch := c.kv.NewBatch()

		raw, err := persist.SaveBlock(batch, genesis)
		if err != nil {
			return err
		}
		if err := persist.SaveTxLocations(batch, genesis.Transactions(), genesis.Header().ID()); err != nil {
			return err
		}
		if err := persist.SaveTrunkBlockID(batch, genesis.Header().ID()); err != nil {
			return err
		}
		if err := persist.SaveBestBlockID(batch, genesis.Header().ID()); err != nil {
			return err
		}
		if err := batch.Write(); err != nil {
			return err
		}
		c.bestBlock.Store(genesis)
		c.caches.block.Add(genesis.Header().ID(), newRawBlock(raw, genesis))
		return nil
	}
	if b0.Header().ID() != genesis.Header().ID() {
		return errors.New("genesis mismatch")
	}
	return nil
}

// AddBlock add a new block into block chain.
// Once reorg happened, Fork.Branch will be the chain transitted from trunk to branch.
// Reorg happens when isTrunk is true.
func (c *Chain) AddBlock(newBlock *block.Block, isTrunk bool) (*Fork, error) {
	c.rw.Lock()
	defer c.rw.Unlock()

	if _, err := c.getBlock(newBlock.Header().ID()); err != nil {
		if !c.IsNotFound(err) {
			return nil, err
		}
	} else {
		// block already there
		return nil, errBlockExist
	}

	if _, err := c.getBlock(newBlock.Header().ParentID()); err != nil {
		if c.IsNotFound(err) {
			return nil, errors.New("parent missing")
		}
		return nil, err
	}

	batch := c.kv.NewBatch()
	raw, err := persist.SaveBlock(batch, newBlock)
	if err != nil {
		return nil, err
	}
	var fork *Fork
	var trunkUpdates map[thor.Hash]bool
	if isTrunk {
		trunkUpdates = make(map[thor.Hash]bool)
		best, err := c.getBestBlock()
		if err != nil {
			return nil, err
		}
		if fork, err = c.buildFork(newBlock, best); err != nil {
			return nil, err
		}

		for _, bb := range fork.Branch {
			if err := persist.EraseTrunkBlockID(batch, bb.Header().ID()); err != nil {
				return nil, err
			}
			if err := persist.EraseTxLocations(batch, bb.Transactions()); err != nil {
				return nil, err
			}
			trunkUpdates[bb.Header().ID()] = false
		}

		for _, tb := range fork.Trunk {
			if err := persist.SaveTrunkBlockID(batch, tb.Header().ID()); err != nil {
				return nil, err
			}
			if err := persist.SaveTxLocations(batch, tb.Transactions(), tb.Header().ID()); err != nil {
				return nil, err
			}
			trunkUpdates[tb.Header().ID()] = true
		}
		persist.SaveBestBlockID(batch, newBlock.Header().ID())
	} else {
		fork = &Fork{Ancestor: newBlock}
	}

	if err := batch.Write(); err != nil {
		return nil, err
	}

	c.caches.block.Add(newBlock.Header().ID(), newRawBlock(raw, newBlock))
	if trunkUpdates != nil {
		for id, f := range trunkUpdates {
			if !f {
				c.caches.trunkBlockID.Remove(block.Number(id))
			}
		}
		for id, f := range trunkUpdates {
			if f {
				c.caches.trunkBlockID.Add(block.Number(id), id)
			}
		}
	}

	if isTrunk {
		c.bestBlock.Store(newBlock)
	}
	return fork, nil
}

// Think about the example below:
//
//   B1--B2--B3--B4--B5--B6
//             \
//              \
//               b4--b5
//
// When call buildFork(B6, b5), the return values will be:
// ((B3, [B6, B5, B4], [b5, b4]), nil)
func (c *Chain) buildFork(trunkHead *block.Block, branchHead *block.Block) (*Fork, error) {
	var (
		trunk, branch []*block.Block
		err           error
		b1            = trunkHead
		b2            = branchHead
	)

	for {
		if b1.Header().Number() > b2.Header().Number() {
			trunk = append(trunk, b1)
			if b1, err = c.getBlock(b1.Header().ParentID()); err != nil {
				return nil, err
			}
			continue
		}
		if b1.Header().Number() < b2.Header().Number() {
			branch = append(branch, b2)
			if b2, err = c.getBlock(b2.Header().ParentID()); err != nil {
				return nil, err
			}
			continue
		}
		if b1.Header().ID() == b2.Header().ID() {
			return &Fork{b1, trunk, branch}, nil
		}

		trunk = append(trunk, b1)
		branch = append(branch, b2)

		if b1, err = c.getBlock(b1.Header().ParentID()); err != nil {
			return nil, err
		}

		if b2, err = c.getBlock(b2.Header().ParentID()); err != nil {
			return nil, err
		}
	}
}

// GetBlockHeader get block header by block id.
func (c *Chain) GetBlockHeader(id thor.Hash) (*block.Header, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockHeader(id)
}

func (c *Chain) getBlockHeader(id thor.Hash) (*block.Header, error) {
	block, err := c.caches.block.GetOrLoad(id)
	if err != nil {
		return nil, err
	}
	header, err := block.(*rawBlock).Header()
	if err != nil {
		return nil, err
	}
	return header, nil
}

// GetBlockBody get block body by block id.
func (c *Chain) GetBlockBody(id thor.Hash) (*block.Body, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockBody(id)
}

func (c *Chain) getBlockBody(id thor.Hash) (*block.Body, error) {
	block, err := c.caches.block.GetOrLoad(id)
	if err != nil {
		return nil, err
	}
	body, err := block.(*rawBlock).Body()
	if err != nil {
		return nil, err
	}
	return body, nil
}

// GetBlock get block by id.
func (c *Chain) GetBlock(id thor.Hash) (*block.Block, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	return c.getBlock(id)
}

func (c *Chain) getBlock(id thor.Hash) (*block.Block, error) {
	block, err := c.caches.block.GetOrLoad(id)
	if err != nil {
		return nil, err
	}
	return block.(*rawBlock).Block()
}

// GetRawBlock get raw block for given id.
// Never modify the returned raw block.
func (c *Chain) GetRawBlock(id thor.Hash) (block.Raw, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	return c.getRawBlock(id)
}

func (c *Chain) getRawBlock(id thor.Hash) (block.Raw, error) {
	block, err := c.caches.block.GetOrLoad(id)
	if err != nil {
		return nil, err
	}
	return block.(*rawBlock).raw, nil
}

// GetBlockIDByNumber returns block id by block number on trunk.
func (c *Chain) GetBlockIDByNumber(num uint32) (thor.Hash, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockIDByNumber(num)
}

func (c *Chain) getBlockIDByNumber(num uint32) (thor.Hash, error) {
	id, err := c.caches.trunkBlockID.GetOrLoad(num)
	if err != nil {
		return thor.Hash{}, err
	}
	return id.(thor.Hash), nil
}

// GetBlockByNumber get block on trunk by its number.
func (c *Chain) GetBlockByNumber(num uint32) (*block.Block, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockByNumber(num)
}

func (c *Chain) getBlockByNumber(num uint32) (*block.Block, error) {
	id, err := c.getBlockIDByNumber(num)
	if err != nil {
		return nil, err
	}
	return c.getBlock(id)
}

// GetRawBlockByNumber get block on trunk by its number.
// Never modify the returned raw block.
func (c *Chain) GetRawBlockByNumber(num uint32) (block.Raw, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	id, err := c.getBlockIDByNumber(num)
	if err != nil {
		return nil, err
	}
	return c.getRawBlock(id)
}

// GetBlockHeaderByNumber get block header on trunk by its number.
func (c *Chain) GetBlockHeaderByNumber(num uint32) (*block.Header, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	id, err := c.getBlockIDByNumber(num)
	if err != nil {
		return nil, err
	}
	return c.getBlockHeader(id)
}

// GetBestBlock get the newest block on trunk.
func (c *Chain) GetBestBlock() (*block.Block, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	return c.getBestBlock()
}

func (c *Chain) getBestBlock() (*block.Block, error) {

	if best := c.bestBlock.Load(); best != nil {
		return best.(*block.Block), nil
	}
	id, err := persist.LoadBestBlockID(c.kv)
	if err != nil {
		return nil, err
	}
	best, err := c.getBlock(id)
	if err != nil {
		return nil, err
	}
	c.bestBlock.Store(best)
	return best, nil
}

// GetTransaction get transaction by id on trunk.
func (c *Chain) GetTransaction(txID thor.Hash) (*tx.Transaction, *persist.TxLocation, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	return c.getTransaction(txID)
}

func (c *Chain) getTransaction(txID thor.Hash) (*tx.Transaction, *persist.TxLocation, error) {
	loc, err := persist.LoadTxLocation(c.kv, txID)
	if err != nil {
		return nil, nil, err
	}
	body, err := c.getBlockBody(loc.BlockID)
	if err != nil {
		return nil, nil, err
	}
	return body.Txs[loc.Index], loc, nil
}

func (c *Chain) getTransactionIDs(blockID thor.Hash) (map[thor.Hash]int, error) {
	ids, err := c.caches.txIDs.GetOrLoad(blockID)
	if err != nil {
		return nil, err
	}
	return ids.(map[thor.Hash]int), nil
}

// LookupTransaction find out the location of a tx, on the chain which ends with blockID.
func (c *Chain) LookupTransaction(blockID thor.Hash, txID thor.Hash) (*persist.TxLocation, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	best, err := c.getBestBlock()
	if err != nil {
		return nil, err
	}
	from, err := c.getBlock(blockID)
	if err != nil {
		return nil, err
	}
	fork, err := c.buildFork(best, from)
	if err != nil {
		return nil, err
	}

	loc, err := persist.LoadTxLocation(c.kv, txID)
	if err != nil {
		if c.IsNotFound(err) {
			for _, b := range fork.Branch {
				ids, err := c.getTransactionIDs(b.Header().ID())
				if err != nil {
					return nil, err
				}
				if index, found := ids[txID]; found {
					return &persist.TxLocation{
						BlockID: b.Header().ID(),
						Index:   uint64(index),
					}, nil
				}
			}
			return nil, errNotFound
		}
		return nil, err
	}
	if block.Number(loc.BlockID) <= fork.Ancestor.Header().Number() {
		return loc, nil
	}
	return nil, errNotFound
}

// SetBlockReceipts set tx receipts of a block.
func (c *Chain) SetBlockReceipts(blockID thor.Hash, receipts tx.Receipts) error {
	c.rw.Lock()
	defer c.rw.Unlock()
	return c.setBlockReceipts(blockID, receipts)
}

func (c *Chain) setBlockReceipts(blockID thor.Hash, receipts tx.Receipts) error {
	c.caches.receipts.Add(blockID, receipts)
	return persist.SaveBlockReceipts(c.kv, blockID, receipts)
}

// GetBlockReceipts get tx receipts of a block.
func (c *Chain) GetBlockReceipts(blockID thor.Hash) (tx.Receipts, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockReceipts(blockID)
}

func (c *Chain) getBlockReceipts(blockID thor.Hash) (tx.Receipts, error) {
	receipts, err := c.caches.receipts.GetOrLoad(blockID)
	if err != nil {
		return nil, err
	}
	return receipts.(tx.Receipts), nil
}

// GetTransactionReceipt get receipt for given tx ID.
func (c *Chain) GetTransactionReceipt(txID thor.Hash) (*tx.Receipt, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getTransactionReceipt(txID)
}

func (c *Chain) getTransactionReceipt(txID thor.Hash) (*tx.Receipt, error) {
	_, loc, err := c.getTransaction(txID)
	if err != nil {
		return nil, err
	}
	receipts, err := c.getBlockReceipts(loc.BlockID)
	if err != nil {
		return nil, err
	}
	return receipts[loc.Index], nil
}

// IsNotFound returns if an error means not found.
func (c *Chain) IsNotFound(err error) bool {
	return err == errNotFound || c.kv.IsNotFound(err)
}

// IsBlockExist returns if the error means block was already in the chain.
func (c *Chain) IsBlockExist(err error) bool {
	return err == errBlockExist
}

// NewTraverser create a block traverser to access blocks on the chain defined by headID.
func (c *Chain) NewTraverser(headID thor.Hash) *Traverser {
	return &Traverser{headID, c, nil}
}

package chain

import (
	"errors"
	"sync"

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
	kv           kv.GetPutter
	genesisBlock *block.Block
	bestBlock    *block.Block
	tag          byte
	caches       caches
	rw           sync.RWMutex
}

type caches struct {
	rawBlocks     *cache
	txIDs         *cache
	receipts      *cache
	trunkBlockIDs *cache
}

// New create an instance of Chain.
func New(kv kv.GetPutter, genesisBlock *block.Block) (*Chain, error) {
	if genesisBlock.Header().Number() != 0 {
		return nil, errors.New("genesis number != 0")
	}
	if genesisID, err := persist.LoadTrunkBlockID(kv, 0); err != nil {
		if !kv.IsNotFound(err) {
			return nil, err
		}
		// no genesis yet
		batch := kv.NewBatch()
		if _, err := persist.SaveBlock(batch, genesisBlock); err != nil {
			return nil, err
		}
		if err := persist.SaveTrunkBlockID(batch, genesisBlock.Header().ID()); err != nil {
			return nil, err
		}
		if err := persist.SaveBestBlockID(batch, genesisBlock.Header().ID()); err != nil {
			return nil, err
		}
		if err := batch.Write(); err != nil {
			return nil, err
		}
	} else if genesisID != genesisBlock.Header().ID() {
		return nil, errors.New("genesis mismatch")
	}
	bestBlockID, err := persist.LoadBestBlockID(kv)
	if err != nil {
		return nil, err
	}

	raw, err := persist.LoadRawBlock(kv, bestBlockID)
	if err != nil {
		return nil, err
	}
	bestBlock, err := (&rawBlock{raw: raw}).Block()
	if err != nil {
		return nil, err
	}

	rawBlocksCache := newCache(blockCacheLimit, func(key interface{}) (interface{}, error) {
		raw, err := persist.LoadRawBlock(kv, key.(thor.Bytes32))
		if err != nil {
			return nil, err
		}
		return &rawBlock{raw: raw}, nil
	})
	txIDsCache := newCache(blockTxIDsLimit, func(key interface{}) (interface{}, error) {
		block, err := rawBlocksCache.GetOrLoad(key)
		if err != nil {
			return nil, err
		}
		ids := make(map[thor.Bytes32]int)
		body, err := block.(*rawBlock).Body()
		if err != nil {
			return nil, err
		}
		for i, tx := range body.Txs {
			ids[tx.ID()] = i
		}
		return ids, nil
	})

	receiptsCache := newCache(receiptsCacheLimit, func(key interface{}) (interface{}, error) {
		return persist.LoadBlockReceipts(kv, key.(thor.Bytes32))
	})

	trunkBlockIDsCache := newCache(trunkBlockIDCacheLimit, func(key interface{}) (interface{}, error) {
		return persist.LoadTrunkBlockID(kv, key.(uint32))
	})

	return &Chain{
		kv:           kv,
		genesisBlock: genesisBlock,
		bestBlock:    bestBlock,
		tag:          genesisBlock.Header().ID()[31],
		caches: caches{
			rawBlocks:     rawBlocksCache,
			txIDs:         txIDsCache,
			receipts:      receiptsCache,
			trunkBlockIDs: trunkBlockIDsCache,
		},
	}, nil
}

// Tag returns chain tag, which is the last byte of genesis id.
func (c *Chain) Tag() byte {
	return c.tag
}

// GenesisBlock returns genesis block.
func (c *Chain) GenesisBlock() *block.Block {
	return c.genesisBlock
}

// BestBlock returns the newest block on trunk.
func (c *Chain) BestBlock() *block.Block {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.bestBlock
}

// AddBlock add a new block into block chain.
// Once reorg happened, Fork.Branch will be the chain transitted from trunk to branch.
// Reorg happens when isTrunk is true.
func (c *Chain) AddBlock(newBlock *block.Block, receipts tx.Receipts, isTrunk bool) (*Fork, error) {
	c.rw.Lock()
	defer c.rw.Unlock()

	newBlockID := newBlock.Header().ID()

	if _, err := c.getBlock(newBlockID); err != nil {
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
	if err := persist.SaveBlockReceipts(c.kv, newBlockID, receipts); err != nil {
		return nil, err
	}
	var fork *Fork
	var trunkUpdates map[thor.Bytes32]bool
	if isTrunk {
		trunkUpdates = make(map[thor.Bytes32]bool)
		if fork, err = c.buildFork(newBlock, c.bestBlock); err != nil {
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
		persist.SaveBestBlockID(batch, newBlockID)
	} else {
		fork = &Fork{Ancestor: newBlock}
	}

	if err := batch.Write(); err != nil {
		return nil, err
	}

	c.caches.rawBlocks.Add(newBlockID, newRawBlock(raw, newBlock))
	c.caches.receipts.Add(newBlockID, receipts)
	if trunkUpdates != nil {
		for id, f := range trunkUpdates {
			if !f {
				c.caches.trunkBlockIDs.Remove(block.Number(id))
			}
		}
		for id, f := range trunkUpdates {
			if f {
				c.caches.trunkBlockIDs.Add(block.Number(id), id)
			}
		}
	}

	if isTrunk {
		c.bestBlock = newBlock
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
// ((B3, [B4, B5, B6], [b4, b5]), nil)
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
			// reverse trunk and branch
			for i, j := 0, len(trunk)-1; i < j; i, j = i+1, j-1 {
				trunk[i], trunk[j] = trunk[j], trunk[i]
			}
			for i, j := 0, len(branch)-1; i < j; i, j = i+1, j-1 {
				branch[i], branch[j] = branch[j], branch[i]
			}
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

func (c *Chain) getRawBlock(id thor.Bytes32) (*rawBlock, error) {
	raw, err := c.caches.rawBlocks.GetOrLoad(id)
	if err != nil {
		return nil, err
	}
	return raw.(*rawBlock), nil
}

// GetBlockHeader get block header by block id.
func (c *Chain) GetBlockHeader(id thor.Bytes32) (*block.Header, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockHeader(id)
}

func (c *Chain) getBlockHeader(id thor.Bytes32) (*block.Header, error) {
	raw, err := c.getRawBlock(id)
	if err != nil {
		return nil, err
	}
	return raw.Header()
}

// GetBlockBody get block body by block id.
func (c *Chain) GetBlockBody(id thor.Bytes32) (*block.Body, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockBody(id)
}

func (c *Chain) getBlockBody(id thor.Bytes32) (*block.Body, error) {
	raw, err := c.getRawBlock(id)
	if err != nil {
		return nil, err
	}
	return raw.Body()
}

// GetBlock get block by id.
func (c *Chain) GetBlock(id thor.Bytes32) (*block.Block, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlock(id)
}

func (c *Chain) getBlock(id thor.Bytes32) (*block.Block, error) {
	raw, err := c.getRawBlock(id)
	if err != nil {
		return nil, err
	}
	return raw.Block()
}

// GetBlockRaw get block rlp encoded bytes for given id.
// Never modify the returned raw block.
func (c *Chain) GetBlockRaw(id thor.Bytes32) (block.Raw, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	raw, err := c.getRawBlock(id)
	if err != nil {
		return nil, err
	}
	return raw.raw, nil
}

// GetBlockIDByNumber returns block id by block number on trunk.
func (c *Chain) GetBlockIDByNumber(num uint32) (thor.Bytes32, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockIDByNumber(num)
}

func (c *Chain) getBlockIDByNumber(num uint32) (thor.Bytes32, error) {
	id, err := c.caches.trunkBlockIDs.GetOrLoad(num)
	if err != nil {
		return thor.Bytes32{}, err
	}
	return id.(thor.Bytes32), nil
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

// GetBlockRawByNumber get block rlp encoded bytes on trunk by its number.
// Never modify the returned raw block.
func (c *Chain) GetBlockRawByNumber(num uint32) (block.Raw, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	id, err := c.getBlockIDByNumber(num)
	if err != nil {
		return nil, err
	}
	raw, err := c.getRawBlock(id)
	if err != nil {
		return nil, err
	}
	return raw.raw, nil
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

// GetTransaction get transaction by id on trunk.
func (c *Chain) GetTransaction(txID thor.Bytes32) (*tx.Transaction, *persist.TxLocation, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	return c.getTransaction(txID)
}

func (c *Chain) getTransaction(txID thor.Bytes32) (*tx.Transaction, *persist.TxLocation, error) {
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

func (c *Chain) getTransactionIDs(blockID thor.Bytes32) (map[thor.Bytes32]int, error) {
	ids, err := c.caches.txIDs.GetOrLoad(blockID)
	if err != nil {
		return nil, err
	}
	return ids.(map[thor.Bytes32]int), nil
}

// LookupTransaction find out the location of a tx, on the chain which ends with blockID.
func (c *Chain) LookupTransaction(blockID thor.Bytes32, txID thor.Bytes32) (*persist.TxLocation, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	from, err := c.getBlock(blockID)
	if err != nil {
		return nil, err
	}
	fork, err := c.buildFork(c.bestBlock, from)
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

// GetBlockReceipts get tx receipts of a block.
func (c *Chain) GetBlockReceipts(blockID thor.Bytes32) (tx.Receipts, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockReceipts(blockID)
}

func (c *Chain) getBlockReceipts(blockID thor.Bytes32) (tx.Receipts, error) {
	receipts, err := c.caches.receipts.GetOrLoad(blockID)
	if err != nil {
		return nil, err
	}
	return receipts.(tx.Receipts), nil
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
func (c *Chain) NewTraverser(headID thor.Bytes32) *Traverser {
	return &Traverser{headID, c, nil}
}

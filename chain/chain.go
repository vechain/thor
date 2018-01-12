package chain

import (
	"errors"
	"sync"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/chain/persist"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const (
	headerCacheLimit   = 512
	bodyCacheLimit     = 512
	blockTxHashesLimit = 1024
)

var errNotFound = errors.New("not found")

// Chain describes a persistent block chain.
// It's thread-safe.
type Chain struct {
	kv        kv.GetPutter
	bestBlock *block.Block
	cached    cached
	rw        sync.RWMutex
}

type cached struct {
	header        *cache.LRU
	body          *cache.LRU
	blockTxHashes *cache.LRU
}

// New create an instance of Chain.
func New(kv kv.GetPutter) *Chain {
	return &Chain{
		kv: kv,
		cached: cached{
			cache.NewLRU(headerCacheLimit),
			cache.NewLRU(bodyCacheLimit),
			cache.NewLRU(blockTxHashesLimit),
		},
	}
}

// WriteGenesis writes in genesis block.
// It will compare the given genesis block with the existed one. If not the same, an error returned.
func (c *Chain) WriteGenesis(genesis *block.Block) error {
	c.rw.Lock()
	defer c.rw.Unlock()

	b, err := c.getBlockByNumber(0)
	if err != nil {
		if !c.IsNotFound(err) {
			// errors occurred
			return err
		}
		// no genesis yet
		batch := c.kv.NewBatch()

		if err := persist.SaveBlock(batch, genesis); err != nil {
			return err
		}
		if err := persist.SaveTxLocations(batch, genesis); err != nil {
			return err
		}
		if err := persist.SaveTrunkBlockHash(batch, genesis.Hash()); err != nil {
			return err
		}
		if err := persist.SaveBestBlockHash(batch, genesis.Hash()); err != nil {
			return err
		}
		if err := batch.Write(); err != nil {
			return err
		}
		c.bestBlock = genesis
		return nil
	}
	if b.Hash() != genesis.Hash() {
		return errors.New("genesis mismatch")
	}
	return nil
}

// AddBlock add a new block into block chain.
// The method will return nil immediately if the block already in the chain.
func (c *Chain) AddBlock(newBlock *block.Block, isTrunk bool) error {
	c.rw.Lock()
	defer c.rw.Unlock()

	if _, err := c.getBlock(newBlock.Hash()); err != nil {
		if !c.IsNotFound(err) {
			return err
		}
	} else {
		// block already there
		return nil
	}

	if _, err := c.getBlock(newBlock.ParentHash()); err != nil {
		if c.IsNotFound(err) {
			return errors.New("parent missing")
		}
		return err
	}

	batch := c.kv.NewBatch()
	if err := persist.SaveBlock(batch, newBlock); err != nil {
		return err
	}

	if isTrunk {
		best, err := c.getBestBlock()
		if err != nil {
			return err
		}

		_, oldBlocks, newBlocks, err := c.traceBackToCommonAncestor(best, newBlock)
		if err != nil {
			return err
		}
		for _, ob := range oldBlocks {
			if err := persist.EraseTrunkBlockHash(batch, ob.Hash()); err != nil {
				return err
			}
			if err := persist.EraseTxLocations(batch, ob); err != nil {
				return err
			}
		}

		for _, nb := range newBlocks {
			if err := persist.SaveTrunkBlockHash(batch, nb.Hash()); err != nil {
				return err
			}
			if err := persist.SaveTxLocations(batch, nb); err != nil {
				return err
			}
		}
		persist.SaveBestBlockHash(batch, newBlock.Hash())
	}

	if err := batch.Write(); err != nil {
		return err
	}

	c.cached.header.Add(newBlock.Hash(), newBlock.Header())
	c.cached.body.Add(newBlock.Hash(), newBlock.Body())

	if isTrunk {
		c.bestBlock = newBlock
	}
	return nil
}

// Think about the example below:
//
//   B1--B2--B3--B4--B5--B6
//             \
//              \
//               b4--b5
//
// When call traceBackToCommonAncestor(B6, b5), the return values will be:
// ([B5, B6, B4], [b5, b4], B3, nil)
func (c *Chain) traceBackToCommonAncestor(b1 *block.Block, b2 *block.Block) (*block.Block, []*block.Block, []*block.Block, error) {
	var (
		c1, c2 []*block.Block
		err    error
	)

	for {
		if b1.Number() > b2.Number() {
			c1 = append(c1, b1)
			if b1, err = c.getBlock(b1.ParentHash()); err != nil {
				return nil, nil, nil, err
			}
			continue
		}
		if b1.Number() < b2.Number() {
			c2 = append(c2, b2)
			if b2, err = c.getBlock(b2.ParentHash()); err != nil {
				return nil, nil, nil, err
			}
			continue
		}
		if b1.Hash() == b2.Hash() {
			return b1, c1, c2, nil
		}

		c1 = append(c1, b1)
		c2 = append(c2, b2)

		if b1, err = c.getBlock(b1.ParentHash()); err != nil {
			return nil, nil, nil, err
		}

		if b2, err = c.getBlock(b2.ParentHash()); err != nil {
			return nil, nil, nil, err
		}
	}
}

// GetBlockHeader get block header by hash of block.
func (c *Chain) GetBlockHeader(hash thor.Hash) (*block.Header, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockHeader(hash)
}

func (c *Chain) getBlockHeader(hash thor.Hash) (*block.Header, error) {
	header, err := c.cached.header.GetOrLoad(hash, func(interface{}) (interface{}, error) {
		return persist.LoadBlockHeader(c.kv, hash)
	})
	if err != nil {
		return nil, err
	}
	return header.(*block.Header), nil
}

// GetBlockBody get block body by hash of block.
func (c *Chain) GetBlockBody(hash thor.Hash) (*block.Body, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockBody(hash)
}

func (c *Chain) getBlockBody(hash thor.Hash) (*block.Body, error) {
	body, err := c.cached.body.GetOrLoad(hash, func(interface{}) (interface{}, error) {
		return persist.LoadBlockBody(c.kv, hash)
	})
	if err != nil {
		return nil, err
	}
	return body.(*block.Body), nil
}

// GetBlock get block by hash.
func (c *Chain) GetBlock(hash thor.Hash) (*block.Block, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	return c.getBlock(hash)
}

func (c *Chain) getBlock(hash thor.Hash) (*block.Block, error) {
	header, err := c.getBlockHeader(hash)
	if err != nil {
		return nil, err
	}
	body, err := c.getBlockBody(hash)
	if err != nil {
		return nil, err
	}
	return block.New(header, body.Txs), nil
}

// GetBlockHashByNumber returns block hash by block number on trunk.
func (c *Chain) GetBlockHashByNumber(num uint32) (thor.Hash, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockHashByNumber(num)
}

func (c *Chain) getBlockHashByNumber(num uint32) (thor.Hash, error) {
	hash, err := persist.LoadTrunkBlockHash(c.kv, num)
	if err != nil {
		return thor.Hash{}, err
	}
	return hash, nil
}

// GetBlockByNumber get block on trunk by its number.
func (c *Chain) GetBlockByNumber(num uint32) (*block.Block, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	return c.getBlockByNumber(num)
}

func (c *Chain) getBlockByNumber(num uint32) (*block.Block, error) {
	hash, err := c.getBlockHashByNumber(num)
	if err != nil {
		return nil, err
	}
	return c.getBlock(hash)
}

// GetBestBlock get the newest block on trunk.
func (c *Chain) GetBestBlock() (*block.Block, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	return c.getBestBlock()
}

func (c *Chain) getBestBlock() (*block.Block, error) {

	if best := c.bestBlock; best != nil {
		return best, nil
	}
	hash, err := persist.LoadBestBlockHash(c.kv)
	if err != nil {
		return nil, err
	}
	best, err := c.getBlock(hash)
	if err != nil {
		return nil, err
	}
	c.bestBlock = best
	return best, nil
}

// GetTransaction get transaction by hash on trunk.
func (c *Chain) GetTransaction(txHash thor.Hash) (*tx.Transaction, *persist.TxLocation, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	tx, loc, err := persist.LoadTx(c.kv, txHash)
	if err != nil {
		return nil, nil, err
	}
	return tx, loc, nil
}

func (c *Chain) getTransactionHashes(blockHash thor.Hash) (map[thor.Hash]int, error) {
	hashes, err := c.cached.blockTxHashes.GetOrLoad(blockHash, func(interface{}) (interface{}, error) {
		body, err := c.getBlockBody(blockHash)
		if err != nil {
			return nil, err
		}
		hashes := make(map[thor.Hash]int)
		for i, tx := range body.Txs {
			hashes[tx.Hash()] = i
		}
		return hashes, nil
	})
	if err != nil {
		return nil, err
	}
	return hashes.(map[thor.Hash]int), nil
}

// LookupTransaction find out the location of a tx, on the chain which ends with blockHash.
func (c *Chain) LookupTransaction(blockHash thor.Hash, txHash thor.Hash) (*persist.TxLocation, error) {
	c.rw.RLock()
	defer c.rw.RUnlock()

	best, err := c.getBestBlock()
	if err != nil {
		return nil, err
	}
	from, err := c.getBlock(blockHash)
	if err != nil {
		return nil, err
	}
	ancestor, branch, _, err := c.traceBackToCommonAncestor(from, best)
	if err != nil {
		return nil, err
	}
	for _, b := range branch {
		hashes, err := c.getTransactionHashes(b.Hash())
		if err != nil {
			return nil, err
		}
		if index, found := hashes[txHash]; found {
			return &persist.TxLocation{
				BlockHash: b.Hash(),
				Index:     uint64(index),
			}, nil
		}
	}
	loc, err := persist.LoadTxLocation(c.kv, txHash)
	if err != nil {
		return nil, err
	}
	if block.Number(loc.BlockHash) <= ancestor.Number() {
		return loc, nil
	}
	return nil, errNotFound
}

// IsNotFound returns if an error means not found.
func (c *Chain) IsNotFound(err error) bool {
	return err == errNotFound || c.kv.IsNotFound(err)
}

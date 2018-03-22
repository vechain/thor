package chain

import (
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/block"
)

type cache struct {
	*lru.ARCCache
	loader func(key interface{}) (interface{}, error)
}

func newLRU(maxSize int, loader func(key interface{}) (interface{}, error)) *cache {
	arc, err := lru.NewARC(maxSize)
	if err != nil {
		panic(err)
	}
	return &cache{arc, loader}
}

func (c *cache) GetOrLoad(key interface{}) (interface{}, error) {
	if value, ok := c.Get(key); ok {
		return value, nil
	}
	value, err := c.loader(key)
	if err != nil {
		return nil, err
	}
	c.Add(key, value)
	return value, nil
}

type rawBlock struct {
	raw    block.Raw
	header atomic.Value
	body   atomic.Value
	block  atomic.Value
}

func newRawBlock(raw block.Raw, block *block.Block) *rawBlock {
	rb := &rawBlock{raw: raw}
	rb.header.Store(block.Header())
	rb.body.Store(block.Body())
	rb.block.Store(block)
	return rb
}

func (rb *rawBlock) Header() (*block.Header, error) {
	if cached := rb.header.Load(); cached != nil {
		return cached.(*block.Header), nil
	}

	h, err := rb.raw.DecodeHeader()
	if err != nil {
		return nil, err
	}
	rb.header.Store(h)
	return h, nil
}

func (rb *rawBlock) Body() (*block.Body, error) {
	if cached := rb.body.Load(); cached != nil {
		return cached.(*block.Body), nil
	}
	b, err := rb.raw.DecodeBody()
	if err != nil {
		return nil, err
	}
	rb.body.Store(b)
	return b, nil
}

func (rb *rawBlock) Block() (*block.Block, error) {
	if cached := rb.block.Load(); cached != nil {
		return cached.(*block.Block), nil
	}

	h, err := rb.Header()
	if err != nil {
		return nil, err
	}
	b, err := rb.Body()
	if err != nil {
		return nil, err
	}

	block := block.Compose(h, b.Txs)

	rb.block.Store(block)
	return block, nil
}

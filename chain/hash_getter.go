package chain

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// HashGetter helper class to support vm.Context.GetHash.
type HashGetter struct {
	chain *Chain
	ref   thor.Hash
	err   error
}

// NewHashGetter create a new HashGetter object.
func NewHashGetter(chain *Chain, ref thor.Hash) *HashGetter {
	return &HashGetter{
		chain,
		ref,
		nil,
	}
}

// Error returns error occurred during GetHash calls.
func (h *HashGetter) Error() error {
	return h.err
}

func (h *HashGetter) setError(err error) {
	if h.err == nil {
		h.err = err
	}
}

// GetHash is compliant with vm.Context.GetHash.
func (h *HashGetter) GetHash(num uint32) thor.Hash {

	ref := h.ref

	for {
		refNum := block.Number(ref)
		if num > refNum {
			return thor.Hash{}
		}
		if num == refNum {
			return ref
		}
		header, err := h.chain.GetBlockHeader(ref)
		if err != nil {
			if !h.chain.IsNotFound(err) {
				h.setError(err)
			}
			return thor.Hash{}
		}
		ref = header.ParentHash()
	}
}

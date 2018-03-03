package chain

import (
	"errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// Traverser help to access block header by number on the chain defined by head id.
type Traverser struct {
	headID thor.Hash
	chain  *Chain
	err    error
}

// Error returns error occurred during the whole life-cycle of Traverser.
func (t *Traverser) Error() error {
	return t.err
}

func (t *Traverser) setError(err error) {
	if t.err == nil {
		t.err = err
	}
}

// Get get block header by block number.
func (t *Traverser) Get(num uint32) *block.Header {
	if num > block.Number(t.headID) {
		t.setError(errors.New("block num larger than head num"))
		return &block.Header{}
	}

	var id thor.Hash
	for id = t.headID; block.Number(id) > num; {
		header, err := t.chain.GetBlockHeader(id)
		if err != nil {
			t.setError(err)
			return &block.Header{}
		}
		id = header.ParentID()
	}
	header, err := t.chain.GetBlockHeader(id)
	if err != nil {
		t.setError(err)
		return &block.Header{}
	}
	return header
}

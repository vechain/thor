package bft

import (
	"errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type rtpc struct {
	repo *chain.Repository
	curr *block.Block
}

func newRTPC(repo *chain.Repository) *rtpc {
	return &rtpc{
		repo: repo,
		curr: nil,
	}
}

func (r *rtpc) update(
	newBlock *block.Block,
	latestCommitted *block.Block,
) error {
	// if the current RTPC block is older than the latest block committed locally
	if r.curr.Header().Timestamp() <= latestCommitted.Header().Timestamp() {
		r.curr = nil
		return nil
	}

	branches := r.repo.GetBranches(newBlock.Header().ID())
	if len(branches) != 1 {
		return errors.New("New block is not a branch head")
	}

	// Construct the view containing the lastest received block `newBlock`
	var id thor.Bytes32
	if newBlock.Header().NV() == GenNVforFirstBlock(newBlock.Header().Number()) {
		id = newBlock.Header().ID()
	} else {
		id = newBlock.Header().NV()
	}
	v := newView(branches[0], id)
	if v == nil {
		return nil
	}


	if v.ifHasConflictPC()

	return nil
}

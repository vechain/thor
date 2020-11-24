package bft

import (
	"errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
)

type rtpc struct {
	repo *chain.Repository
	curr *block.Header
}

func newRTPC(repo *chain.Repository) *rtpc {
	return &rtpc{
		repo: repo,
		curr: nil,
	}
}

func (r *rtpc) getRTPC() *block.Header {
	return r.curr
}

func (r *rtpc) updateByLatestCommitted(latestCommitted *block.Header) {
	// if the current RTPC block is older than the latest block committed locally
	if r.curr.Timestamp() <= latestCommitted.Timestamp() {
		r.curr = nil
	}
}

func (r *rtpc) updateByNewBlock(newBlock *block.Block) error {
	var (
		branches []*chain.Chain
		summary  *chain.BlockSummary
		vw       *view
		err      error
	)

	branches = r.repo.GetBranchesByID(newBlock.Header().ID())
	if len(branches) != 1 || branches[0].HeadID() != newBlock.Header().ID() {
		return errors.New("New block is not a branch head")
	}

	// Construct the view containing the lastest received block `newBlock`
	nv := newBlock.Header().NV()
	if newBlock.Header().NV() == GenNVforFirstBlock(newBlock.Header().Number()) {
		nv = newBlock.Header().ID()
	}
	vw, err = newView(branches[0], block.Number(nv))
	if err != nil {
		return err
	}

	// Check whether there are 2f+1 sigs
	if !vw.ifHasQCForNV() {
		return nil
	}

	// Make sure the view is newer
	summary, err = r.repo.GetBlockSummary(nv)
	if err != nil {
		return err
	}
	if r.curr != nil && summary.Header.Timestamp() <= r.curr.Timestamp() {
		return nil
	}

	// Invalidate the current RTPC block if the latest view contains no pc message of the block
	if r.curr != nil && vw.getNumSigOnPC(r.curr.ID()) == 0 {
		r.curr = nil
	}

	// Check whether the view has 2f+1 pp messages and no conflict pc message
	ok, pp := vw.ifHasQCForPP()
	if !ok || vw.ifHasConflictPC() {
		return nil
	}

	// Check whether every newer view contains pc message of the view
	ifUpdate := true
	branches = r.repo.GetBranchesByTimestamp(summary.Header.Timestamp())
	for _, branch := range branches {
		num := block.Number(branch.HeadID())
		for {
			header, err := branch.GetBlockHeader(num)
			if err != nil {
				return err
			}
			num = block.Number(header.NV())
			if num <= block.Number(nv) {
				break
			}

			vw, err := newView(branch, num)
			if err != nil {
				return err
			}

			if vw.getNumSigOnPC(nv) == 0 {
				ifUpdate = false
				goto END_SEARCH
			}
		}
	}
END_SEARCH:

	// update RTPC if every view newer than the view contains at least one pc message for the view
	if ifUpdate {
		summary, err = r.repo.GetBlockSummary(pp)
		if err != nil {
			return err
		}
		r.curr = summary.Header
	}

	return nil
}

package bft

import (
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

func (r *rtpc) updateByLastCommitted(lastCommitted *block.Header) {
	if r.curr == nil {
		return
	}

	// if the current RTPC block is older than the latest block committed locally
	if r.curr.Timestamp() <= lastCommitted.Timestamp() {
		r.curr = nil
	}
}

func (r *rtpc) updateByNewBlock(newBlock *block.Block) error {
	// Construct the view containing the lastest received block `newBlock`
	branch := r.repo.NewChain(newBlock.Header().ID())
	currView, err := newView(branch, block.Number(newBlock.Header().NV()))
	if err != nil {
		return err
	}

	// Check whether there are 2f+1 sigs
	if !currView.hasQCForNV() {
		return nil
	}

	// Make sure the view is newer that the current RTPC block
	summary, err := r.repo.GetBlockSummary(currView.getFirstBlockID())
	currViewTS := summary.Header.Timestamp()
	if err != nil {
		return err
	}
	if r.curr != nil && currViewTS <= r.curr.Timestamp() {
		return nil
	}

	// Invalidate the current RTPC block if the latest view contains no pc message of the block
	if r.curr != nil && currView.getNumSigOnPC(r.curr.ID()) == 0 {
		r.curr = nil
	}

	// Stop here if there is a valid RTPC block
	if r.curr != nil {
		return nil
	}

	// Check whether the view has 2f+1 pp messages and no conflict pc message
	ok, id := currView.hasQCForPP()
	if !ok || currView.hasConflictPC() {
		return nil
	}
	summary, err = r.repo.GetBlockSummary(id)
	if err != nil {
		return err
	}
	candidate := summary.Header

	// Check whether every newer view contains pc message of the candidate RTPC block
	ifUpdate := true
	branches, err := r.repo.GetBranchesByTimestamp(currViewTS)
	if err != nil {
		return err
	}
	for _, branch := range branches {
		num := block.Number(branch.HeadID())
		for {
			header, err := branch.GetBlockHeader(num)
			if err != nil {
				return err
			}
			num = block.Number(header.NV())
			if nv, err := branch.GetBlockHeader(num); err != nil {
				return err
			} else if nv.Timestamp() <= currViewTS {
				break
			}

			vw, err := newView(branch, num)
			if err != nil {
				return err
			}

			if vw.hasQCForNV() && vw.getNumSigOnPC(candidate.ID()) == 0 {
				ifUpdate = false
				goto END_SEARCH
			}

			// move to the previous view
			num = num - 1
			if num <= 0 {
				break
			}
		}
	}
END_SEARCH:

	// update RTPC if every view newer than the view contains at least one pc message for the view
	if ifUpdate {
		r.curr = candidate
	}

	return nil
}

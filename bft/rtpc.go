package bft

import (
	"errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type rtpc struct {
	repo          *chain.Repository
	currRTPC      *block.Header
	currView      *block.Header
	lastCommitted thor.Bytes32
}

func newRTPC(repo *chain.Repository, lastCommitted thor.Bytes32) *rtpc {
	return &rtpc{
		repo:          repo,
		lastCommitted: lastCommitted,
	}
}

func (r *rtpc) get() *block.Header {
	return r.currRTPC
}

func (r *rtpc) updateLastCommitted(lastCommitted thor.Bytes32) error {
	if !r.lastCommitted.IsZero() {
		// lastCommitted must be an offspring of r.lastCommitted
		ok, err := isAncestor(r.repo, lastCommitted, r.lastCommitted)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("Input block must be an offspring of the last committed")
		}
	}

	r.lastCommitted = lastCommitted

	if r.currRTPC == nil {
		return nil
	}

	if blk, err := r.repo.GetBlock(lastCommitted); err != nil {
		return err
	} else if len(blk.BackerSignatures()) < MinNumBackers {
		// check heavy block
		return errors.New("Non-heavy block")
	} else if r.currRTPC.Timestamp() <= blk.Header().Timestamp() {
		// reset the current if earlier than the last committed
		r.currRTPC = nil
		r.currView = nil
	}

	return nil
}

func (r *rtpc) update(newBlock *block.Block) error {
	// Check heavy block
	if len(newBlock.BackerSignatures()) < MinNumBackers {
		return errors.New("Non-heavy block")
	}

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

	// If currRTPC != nil, make sure the view is newer than currView
	summary, err := r.repo.GetBlockSummary(currView.getFirstBlockID())
	currViewBlock := summary.Header
	if err != nil {
		return err
	}
	if r.currRTPC != nil && currViewBlock.Timestamp() <= r.currView.Timestamp() {
		return nil
	}

	// Invalidate the current RTPC block if the latest view contains no pc message of the block
	if r.currRTPC != nil && currView.getNumSigOnPC(r.currRTPC.ID()) == 0 {
		r.currRTPC = nil
	}

	// Stop here if there is a valid RTPC block
	if r.currRTPC != nil {
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
	branches, err := r.repo.GetBranchesByTimestamp(currViewBlock.Timestamp())
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

			// Along each branch, search for views that are newer than the view
			// that includes the new block
			if nv, err := branch.GetBlockHeader(num); err != nil {
				return err
			} else if nv.Timestamp() <= currViewBlock.Timestamp() {
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
		r.currRTPC = candidate
		r.currView = currViewBlock
	}

	return nil
}

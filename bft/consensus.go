package bft

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

// MaxByzantineNodes - Maximum number of Byzatine nodes, i.e., f
const MaxByzantineNodes = 33

// QC = N - f
const QC = int(thor.MaxBlockProposers) - MaxByzantineNodes

// Indices of local state vector
const (
	NV int = iota
	PP
	PC
	CM
	FN
)

// Consensus ...
type Consensus struct {
	repo      *chain.Repository
	state     [5]thor.Bytes32
	committed *committedBlockInfo
	rtpc      *rtpc

	lastSignedPC            thor.Bytes32
	lastSignedPCViewTS      uint64
	hasLastSignedpPCExpired bool

	nodeAddress   thor.Address
	prevBestBlock *block.Header
}

// NewConsensus initializes BFT consensus
func NewConsensus(
	repo *chain.Repository,
	lastFinalized thor.Bytes32,
	nodeAddress thor.Address,
) *Consensus {
	state := [5]thor.Bytes32{}
	state[FN] = lastFinalized

	return &Consensus{
		repo:          repo,
		state:         state,
		committed:     newCommittedBlockInfo(lastFinalized),
		rtpc:          newRTPC(repo, lastFinalized),
		nodeAddress:   nodeAddress,
		prevBestBlock: repo.BestBlock().Header(),
	}
}

// UpdateLastSignedPC updates lastSignedPC.
// This function is called by the leader after he generates a new block or
// by a backer after he backs a block proposal
func (cons *Consensus) UpdateLastSignedPC(h *block.Header) error {
	var (
		viewTS uint64
		err    error
	)

	// PC value must not be zero or equal to the existing lastSignedPC
	if h.PC().IsZero() || h.PC() == cons.lastSignedPC {
		return nil
	}

	// If the block is the first of its view, use the block's timestamp directly.
	// It is because the block may have not yet been added to repo in case of a
	// backer calling this function;
	// Otherwise, the block referred by the NV value should have already existed
	// in repo.
	if h.NV() == GenNVforFirstBlock(h.Number()) {
		viewTS = h.Timestamp()
	} else {
		viewTS, err = getTimestamp(cons.repo, getNV(h))
		if err != nil {
			return err
		}
	}

	// view must be newer
	if viewTS <= cons.lastSignedPCViewTS {
		return nil
	}

	cons.lastSignedPC = h.PC()
	cons.lastSignedPCViewTS = viewTS
	cons.hasLastSignedpPCExpired = false

	return nil
}

// Update updates the local BFT state vector
func (cons *Consensus) Update(newBlock *block.Block) error {
	// Check whether the new block is on the canonical chain
	// Here the new block should have already been added to cons.repo
	best := cons.repo.BestBlock().Header()
	isOnConanicalChain := best.ID() == newBlock.Header().ID()

	branch := cons.repo.NewChain(newBlock.Header().ID())
	v, err := newView(branch, block.Number(newBlock.Header().NV()))
	if err != nil {
		return err
	}

	///////////////
	// update CM //
	///////////////
	// Check whether there are 2f + 1 same pp messages and no conflict pc in the view
	if ok, cm := v.hasQCForPC(); ok && !v.hasConflictPC() {
		cons.state[CM] = cm
		cons.state[PC] = thor.Bytes32{}
		cons.committed.updateLocal(cm)

		// Update RTPC
		if err := cons.rtpc.updateLastCommitted(cons.state[CM]); err != nil {
			return err
		}
	}
	// Check whether there are f+1 same cm messages
	if cm := newBlock.Header().CM(); block.Number(cm) > block.Number(cons.state[CM]) {
		// Check whether there are f+1 cm messages
		if cons.committed.updateObserved(newBlock) {
			cons.state[CM] = cm
			cons.committed.updateLocal(cm)

			// Update RTPC
			if err := cons.rtpc.updateLastCommitted(cons.state[CM]); err != nil {
				return err
			}
		}
	}
	// update the finalized block info
	if block.Number(cons.state[FN]) < block.Number(cons.state[CM]) {
		cons.state[FN] = cons.state[CM]
	}

	///////////////
	// update PC //
	///////////////
	// Update RTPC
	if err := cons.rtpc.update(newBlock); err != nil {
		return err
	}

	// Check whether the current view invalidates the last signed pc
	if !cons.lastSignedPC.IsZero() && v.hasQCForNV() && v.getNumSigOnPC(cons.lastSignedPC) == 0 {
		ts, err := getTimestamp(cons.repo, getNV(newBlock.Header()))
		if err != nil {
			return err
		}
		if ts > cons.lastSignedPCViewTS {
			cons.hasLastSignedpPCExpired = true
		}
	}

	if rtpc := cons.rtpc.get(); rtpc != nil {
		ifUpdatePC := false
		if !cons.lastSignedPC.IsZero() {
			ok, err := cons.repo.IfConflict(rtpc.ID(), cons.lastSignedPC)
			if err != nil {
				return err
			}
			if !ok {
				ifUpdatePC = true
			} else if cons.hasLastSignedpPCExpired {
				ifUpdatePC = true
			}
		} else {
			ifUpdatePC = true
		}

		if ifUpdatePC {
			cons.state[PC] = rtpc.ID()
		}
	}

	///////////////
	// Unlock pc //
	///////////////
	if rtpc := cons.rtpc.get(); rtpc != nil {
		if cons.state[PC] != rtpc.ID() {
			cons.state[PC] = thor.Bytes32{}
		}
	}

	///////////////
	// Update pp //
	///////////////
	if isOnConanicalChain && v.hasQCForNV() && !v.hasConflictPC() {
		cons.state[PP] = v.getFirstBlockID()
	}

	///////////////
	// Update nv //
	///////////////
	if isOnConanicalChain {
		nv := getNV(newBlock.Header())

		if cons.state[NV].IsZero() {
			cons.state[NV] = nv
		} else {
			ts0, err := getTimestamp(cons.repo, cons.state[NV])
			if err != nil {
				return err
			}
			ts1, err := getTimestamp(cons.repo, newBlock.Header().ID())
			if err != nil {
				return err
			}
			if ts1 > ts0 {
				cons.state[NV] = nv
			} else if newBlock.Header().ParentID() != cons.prevBestBlock.ID() {
				cons.state[NV] = newBlock.Header().ID()
			}

			// Check whether the view including the parent of the new block
			// has already obtained 2f+1 nv messages. If yes, then start a
			// new view.
			pid := newBlock.Header().ParentID()
			summary, err := cons.repo.GetBlockSummary(pid)
			if err != nil {
				return err
			}
			w, err := newView(cons.repo.NewChain(pid), block.Number(summary.Header.NV()))
			if err != nil {
				return err
			}
			if w.hasQCForNV() {
				cons.state[NV] = newBlock.Header().ID()
			}
		}
	}

	///////////////
	// unlock pp //
	///////////////
	if !cons.state[NV].IsZero() && !cons.state[PP].IsZero() {
		if ok, err := cons.repo.IfConflict(cons.state[NV], cons.state[PP]); err != nil {
			return err
		} else if ok {
			cons.state[PP] = thor.Bytes32{}
		}
	}

	// update prevBestBlock
	cons.prevBestBlock = best

	return nil
}

// Get returns the local finality state
func (cons *Consensus) Get() [5]thor.Bytes32 {
	return cons.state
}

func isAncestor(repo *chain.Repository, offspring, ancestor thor.Bytes32) (bool, error) {
	if block.Number(offspring) <= block.Number(ancestor) {
		return false, nil
	}

	if _, err := repo.GetBlockSummary(offspring); err != nil {
		return false, err
	}

	branch := repo.NewChain(offspring)
	ok, err := branch.HasBlock(ancestor)
	if err != nil {
		return false, err
	}

	return ok, nil
}

func getTimestamp(repo *chain.Repository, id thor.Bytes32) (uint64, error) {
	summary, err := repo.GetBlockSummary(id)
	if err != nil {
		return 0, err
	}

	return summary.Header.Timestamp(), nil
}

func getNV(h *block.Header) (nv thor.Bytes32) {
	nv = h.NV()
	if block.Number(nv) == block.Number(h.ID()) {
		nv = h.ID()
	}
	return
}

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
	repo                    *chain.Repository
	state                   [5]thor.Bytes32
	committed               *committedBlockInfo
	rtpc                    *rtpc
	hasLastSignedpPCExpired bool
	lastSignedPC            thor.Bytes32

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
		repo:                    repo,
		state:                   state,
		committed:               newCommittedBlockInfo(lastFinalized),
		rtpc:                    newRTPC(repo, lastFinalized),
		hasLastSignedpPCExpired: false,
		lastSignedPC:            thor.Bytes32{},
		nodeAddress:             nodeAddress,
		prevBestBlock:           repo.BestBlock().Header(),
	}
}

// Update updates the local BFT state vector
func (cons *Consensus) Update(newBlock *block.Block) error {
	// update lastSignedPC
	if pc := newBlock.Header().PC(); !pc.IsZero() {
		signers := getSigners(newBlock)
		for _, signer := range signers {
			if signer == cons.nodeAddress {
				cons.lastSignedPC = pc
				cons.hasLastSignedpPCExpired = false
			}
		}
	}

	// Check whether the new block is on the canonical chain
	isOnConanicalChain := false
	best := cons.repo.BestBlock().Header()
	if best.ID() == cons.prevBestBlock.ID() {
		isOnConanicalChain = true
	}

	branch := cons.repo.NewChain(newBlock.Header().ID())
	v, err := newView(branch, block.Number(newBlock.Header().NV()))
	if err != nil {
		return err
	}

	if !v.hasQCForNV() {
		return nil
	}

	///////////////
	// update CM //
	///////////////
	// Check whether there are 2f + 1 same pp messages and no conflict pc in the view
	if ok, cm := v.hasQCForPC(); ok && !v.hasConflictPC() {
		cons.state[CM] = cm
		cons.state[PC] = thor.Bytes32{}
		cons.committed.updateLocal(cm)
	}
	// Check whether there are f+1 same cm messages
	if cm := newBlock.Header().CM(); !cm.IsZero() && cm != cons.state[CM] {
		// Check whether there are f+1 cm messages
		ok := cons.committed.updateObserved(cm)
		if ok {
			cons.state[CM] = cm
			cons.committed.updateLocal(cm)
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
	if !cons.state[CM].IsZero() {
		if err := cons.rtpc.updateLastCommitted(cons.state[CM]); err != nil {
			return err
		}
	}
	if err := cons.rtpc.update(newBlock); err != nil {
		return err
	}

	// Check whether the current view invalidates the last signed pc
	if !cons.lastSignedPC.IsZero() && v.getNumSigOnPC(cons.lastSignedPC) == 0 {
		cons.hasLastSignedpPCExpired = true
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
	if isOnConanicalChain && !v.hasConflictPC() {
		cons.state[PP] = v.getFirstBlockID()
	}

	///////////////
	// Update nv //
	///////////////
	if isOnConanicalChain {
		nv := newBlock.Header().NV()
		if block.Number(nv) == newBlock.Header().Number() {
			nv = newBlock.Header().ID()
		}

		if cons.state[NV].IsZero() {
			cons.state[NV] = nv
		} else {
			summary, err := cons.repo.GetBlockSummary(cons.state[NV])
			if err != nil {
				return err
			}
			if newBlock.Header().Timestamp() > summary.Header.Timestamp() {
				cons.state[NV] = nv
			} else if newBlock.Header().ParentID() != cons.prevBestBlock.ID() {
				cons.state[NV] = newBlock.Header().ID()
			}

			// Check whether the view including the parent of the new block
			// has already obtained 2f+1 nv messages. If yes, then start a
			// new view.
			pid := newBlock.Header().ParentID()
			summary, err = cons.repo.GetBlockSummary(pid)
			if err != nil {
				return err
			}
			first := block.Number(summary.Header.NV())
			w, err := newView(cons.repo.NewChain(pid), first)
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

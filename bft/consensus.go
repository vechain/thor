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
	repo               *chain.Repository
	state              [5]thor.Bytes32
	committed          *committedBlockInfo
	rtpc               *rtpc
	hasPCSignedExpired bool
	lastSigned         *block.Header

	nodeAddress thor.Address
	prevBest    *block.Header
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
		repo:               repo,
		state:              state,
		committed:          newCommittedBlockInfo(lastFinalized),
		rtpc:               newRTPC(repo, lastFinalized),
		hasPCSignedExpired: false,
		lastSigned:         nil,
		nodeAddress:        nodeAddress,
		prevBest:           repo.BestBlock().Header(),
	}
}

// Update updates the local BFT state vector
func (cons *Consensus) Update(newBlock *block.Block) error {
	// update lastSigned
	signers := getSigners(newBlock)
	for _, signer := range signers {
		if signer == cons.nodeAddress {
			cons.lastSigned = newBlock.Header()
		}
	}

	// Check whether the new block is on the canonical chain
	isOnConanicalChain := false
	best := cons.repo.BestBlock().Header()
	if best.ID() == cons.prevBest.ID() {
		isOnConanicalChain = true
	}

	branch := cons.repo.NewChain(newBlock.Header().ID())
	v, err := newView(branch, block.Number(newBlock.Header().NV()))
	if err != nil {
		return err
	}

	// update CM
	if ok, cm := v.hasQCForPC(); ok && !v.hasConflictPC() {
		cons.state[CM] = cm
		cons.state[PC] = thor.Bytes32{}
		cons.committed.updateLocal(cm)
	}
	if cm := newBlock.Header().CM(); !cm.IsZero() {
		// Check whether there are f+1 cm messages
		ok1, err := cons.committed.updateObserved(cm)
		if err != nil {
			return err
		}

		if ok1 {
			ifUpdate := true
			// Check whether the new cm is an offspring of the old cm
			if !cons.state[CM].IsZero() {
				ok2, err := isAncestor(cons.repo, cm, cons.state[CM])
				if err != nil {
					return err
				}
				if !ok2 {
					ifUpdate = false
				}
			}

			if ifUpdate {
				cons.state[CM] = cm
				cons.committed.updateLocal(cm)
			}
		}
	}
	if block.Number(cons.state[FN]) < block.Number(cons.state[CM]) {
		cons.state[FN] = cons.state[CM]
	}

	// update PC
	if !cons.state[CM].IsZero() {
		cons.rtpc.updateLastCommitted(cons.state[CM])
	}
	if cons.lastSigned != nil {
		if v.hasQCForNV() && v.getNumSigOnPC(cons.lastSigned.PC()) == 0 {
			cons.hasPCSignedExpired = true
		}
	}
	if rtpc := cons.rtpc.get(); rtpc != nil {
		ifUpdatePC := false
		if cons.lastSigned != nil {
			ok, err := cons.repo.IfConflict(rtpc.ID(), cons.lastSigned.PC())
			if err != nil {
				return err
			}
			if !ok {
				ifUpdatePC = true
			} else if cons.hasPCSignedExpired {
				ifUpdatePC = true
			}
		} else {
			ifUpdatePC = true
		}

		if ifUpdatePC {
			cons.state[PC] = rtpc.ID()
		}
	}

	// Unlock pc
	if rtpc := cons.rtpc.get(); rtpc != nil {
		if cons.state[PC] != rtpc.ID() {
			cons.state[PC] = thor.Bytes32{}
		}
	}

	// Update pp
	if isOnConanicalChain {
		if v.hasQCForNV() && !v.hasConflictPC() {
			cons.state[PP] = v.getFirstBlockID()
		}
	}

	// Update nv
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
			} else if newBlock.Header().ParentID() != cons.prevBest.ID() {
				cons.state[NV] = newBlock.Header().ID()
			}

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

	// unlock pp
	if ok, err := cons.repo.IfConflict(cons.state[NV], cons.state[PP]); err != nil {
		return err
	} else if ok {
		cons.state[PP] = thor.Bytes32{}
	}

	// update prevBest
	cons.prevBest = best

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

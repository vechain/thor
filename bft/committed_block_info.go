package bft

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

type committedBlockInfo struct {
	local    thor.Bytes32
	observed map[thor.Bytes32]map[thor.Address]uint8
}

func newCommittedBlockInfo(id thor.Bytes32) *committedBlockInfo {
	return &committedBlockInfo{
		local:    id,
		observed: make(map[thor.Bytes32]map[thor.Address]uint8),
	}
}

// updateLocal updates the latest localled committed block
func (info *committedBlockInfo) updateLocal(id thor.Bytes32) {
	if block.Number(id) <= block.Number(info.local) {
		return
	}

	info.local = id

	// remove blocks committed by others that have lower block numbers
	for k := range info.observed {
		if block.Number(k) <= block.Number(id) {
			delete(info.observed, k)
		}
	}
}

// updateObserved updates observed blocks committed by other nodes. It returns true
// if the input block is committed by at least f+1 nodes.
func (info *committedBlockInfo) updateObserved(b *block.Block) bool {
	// Get cm value
	cm := b.Header().CM()

	// Check height
	if block.Number(cm) <= block.Number(info.local) {
		return false
	}

	// Init map
	if _, ok := info.observed[cm]; !ok {
		info.observed[cm] = make(map[thor.Address]uint8)
	}

	// Update the observed info
	signers := getSigners(b)
	for _, signer := range signers {
		info.observed[cm][signer] = info.observed[cm][signer] + 1
	}

	// Check whether there are f+1 cm messages
	if len(info.observed[cm]) >= MaxByzantineNodes+1 {
		return true
	}

	return false
}

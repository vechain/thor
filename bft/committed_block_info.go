package bft

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

type committedBlockInfo struct {
	local    thor.Bytes32
	observed map[thor.Bytes32]uint8
}

func newCommittedBlockInfo(id thor.Bytes32) *committedBlockInfo {
	return &committedBlockInfo{
		local:    id,
		observed: make(map[thor.Bytes32]uint8),
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
func (info *committedBlockInfo) updateObserved(id thor.Bytes32) bool {
	if block.Number(id) <= block.Number(info.local) {
		return false
	}

	info.observed[id] = info.observed[id] + 1

	if info.observed[id] >= MaxByzantineNodes+1 {
		return true
	}

	return false
}

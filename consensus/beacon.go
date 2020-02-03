package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// beacon computes the random seed for the given epoch
//
// block_timestamp = launch_time + block_interval * round_number
// Round 0 : genesis
// Epoch 1 : round [1, epoch_interval]
// BlockNumber 0 : genesis
func (c *Consensus) beacon(epoch uint32) (thor.Bytes32, error) {
	if epoch == 0 {
		return thor.BytesToBytes32(nil), newConsensusError(trNil, strErrZeroEpoch, nil, nil, "")
	}

	best := c.chain.BestBlock()
	lastRoundOfEpoch := (epoch - 1) * uint32(thor.EpochInterval)
	lastTimestampOfEpoch := c.Timestamp(lastRoundOfEpoch)

	var (
		header *block.Header
		err    error
	)

	// Start the search from the block with its number equal to lastRoundOfEpoch.
	// The actual number may be smaller than lastRoundOfEpoch if there is any
	// round when no block is produced.
	last := lastRoundOfEpoch
	if last > best.Header().Number() {
		last = best.Header().Number()
	}

	header, err = c.chain.GetTrunkBlockHeader(last)
	if err != nil {
		return thor.Bytes32{}, err
	}

	for {
		// Check whether the block is within the epoch
		if header.Timestamp() <= lastTimestampOfEpoch {
			break
		}

		// Get the parent header
		header, err = c.chain.GetBlockHeader(header.ParentID())
		if err != nil {
			panic("Parent not found")
		}
	}

	return compBeacon(header), nil
}

// beacon = hash(concat(header.VrfProofs()...))
func compBeacon(header *block.Header) thor.Bytes32 {
	var beacon thor.Bytes32

	hw := thor.NewBlake2b()
	for _, proof := range header.VrfProofs() {
		hw.Write(proof.Bytes())
	}
	hw.Sum(beacon[:0])

	return beacon
}

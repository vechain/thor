package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// beacon computes the epoch beacon beacon
//
// block_timestamp == launch_time + block_interval * round_number
// round 0 == genesis
// epoch 1 == [1, epoch_interval] (round)
// block number of genesis == 0
func (c *Consensus) beacon(epoch uint32) (beacon thor.Bytes32, err error) {
	// epoch number must be greater than zero
	if epoch == 0 {
		return thor.BytesToBytes32(nil), newConsensusError(trNil, strErrZeroEpoch, nil, nil, "")
	}

	// cache
	if beacon, ok := c.beaconCache.Get(epoch); ok {
		return beacon.(thor.Bytes32), nil
	}
	defer func() {
		if err == nil {
			c.beaconCache.Add(epoch, beacon)
		}
	}()

	var (
		header    *block.Header
		best      = c.repo.BestBlock()
		lastRound = (epoch - 1) * uint32(thor.EpochInterval)
	)

	// Start the search from the block with its number equal to [last].
	// The actual number may be smaller than lastRound if there is any
	// round when no block is produced. Therefore, we choose
	//
	// min(lastRound, bestBlockNum)
	//
	// as the searching starting point
	last := lastRound
	if last > best.Header().Number() {
		last = best.Header().Number()
	}

	header, err = c.repo.NewBestChain().GetBlockHeader(last)
	if err != nil {
		return thor.Bytes32{}, err
	}

	for {
		// Check whether the block is valid
		if header.Timestamp() <= c.Timestamp(lastRound) {
			break
		}

		// Get the parent header
		s, err := c.repo.GetBlockSummary(header.ParentID())
		if err != nil {
			panic("Parent not found")
		}
		header = s.Header
	}

	beacon = compBeacon(header)
	return
}

// beacon = hash(vrf_proof1 || vrf_proof2 || ...)
func compBeacon(header *block.Header) thor.Bytes32 {
	var beacon thor.Bytes32

	hw := thor.NewBlake2b()
	for _, proof := range header.VrfProofs() {
		hw.Write(proof.Bytes())
	}
	hw.Sum(beacon[:0])

	return beacon
}

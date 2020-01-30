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
		return thor.BytesToBytes32(nil), consensusError("Cannot be epoch zero")
	}

	best := c.chain.BestBlock()
	// bestRound, _ := c.RoundNumber(best.Header().Timestamp())
	// bestEpoch := EpochNumber(bestRound)
	// if bestRound == 0 {
	// 	return getBeaconFromHeader(c.chain.GenesisBlock().Header()), nil
	// }
	// if epoch > bestEpoch+1 {
	// 	return thor.BytesToBytes32(nil), errFutureEpoch
	// }

	lastRound := (epoch - 1) * uint32(thor.EpochInterval)

	var (
		header *block.Header
		err    error
		round  uint32
	)

	last := lastRound
	if last > best.Header().Number() {
		last = best.Header().Number()
	}
	header, err = c.chain.GetTrunkBlockHeader(last)
	if err != nil {
		return thor.Bytes32{}, err
	}

	// Backtrack from the last round of the epoch to extract the last block
	// within the epoch
	// for i := last; i >= 0; i-- {
	// header, err = c.chain.GetTrunkBlockHeader(i)
	// 	if err == nil {
	// 		break
	// 	}
	// }

	// if err != nil {
	// 	panic("No block found")
	// }

	for {
		// if header.Number() == 0 {
		// 	break
		// }

		round = c.RoundNumber(header.Timestamp())
		if round <= lastRound {
			break
		}

		header, err = c.chain.GetBlockHeader(header.ParentID())
		if err != nil {
			// hex := hex.EncodeToString(header.ParentID().Bytes())
			// return thor.BytesToBytes32(nil), errors.New("Block " + hex + " not found")
			panic("Parent not found")
		}
	}

	return getBeaconFromHeader(header), nil
}

func getBeaconFromHeader(header *block.Header) thor.Bytes32 {
	return header.ID()
}

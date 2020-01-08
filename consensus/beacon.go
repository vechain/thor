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
		return thor.BytesToBytes32(nil), errZeroEpoch
	}

	if c.chain.BestBlock().Header().Number() == 0 {
		return thor.BytesToBytes32(nil), errZeroChain
	}

	bestRound, _ := c.RoundNumber(c.chain.BestBlock().Header().Timestamp())
	bestEpoch, _ := EpochNumber(bestRound)
	if epoch > bestEpoch {
		return thor.BytesToBytes32(nil), errFutureEpoch
	}

	// Get the potential number of the last block in the last epoch
	lastRound := (epoch - 1) * uint32(thor.EpochInterval)

	// For the first epoch, use the id of the genesis block as the seed
	if lastRound == 0 {
		return getBeaconFromHeader(c.chain.GenesisBlock().Header()), nil
	}

	var (
		header *block.Header
		err    error
		round  uint32
	)

	// Backtrack from the last round of the epoch to extract the last block
	// within the epoch
	for i := uint32(0); i < lastRound; i++ {
		header, err = c.chain.GetTrunkBlockHeader(lastRound - i)
		if err == nil {
			break
		}
	}

	if err != nil {
		panic("No block found")
	}

	for {
		if header.Number() == 0 {
			break
		}

		round, _ = c.RoundNumber(header.Timestamp())
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

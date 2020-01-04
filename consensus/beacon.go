package consensus

import (
	"encoding/hex"
	"errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// Beacon computes the random seed for the given epoch
//
// block_timestamp = launch_time + block_interval * round_number
// Round 0 : genesis
// Epoch 1 : round [1, epoch_interval]
// BlockNumber 0 : genesis
func (c *Consensus) Beacon(currEpochNum uint32) (thor.Bytes32, error) {
	// Get the potential number of the last block in the last epoch
	lastRound := (currEpochNum - 1) * uint32(thor.EpochInterval)

	// For the first epoch, use the id of the genesis block as the seed
	if lastRound == 0 {
		return c.chain.GenesisBlock().Header().ID(), nil
	}

	var (
		header *block.Header
		err    error
	)

	launchTime := c.chain.GenesisBlock().Header().Timestamp()

	// Backtrack the first block starting from height lastRound
	r := lastRound
	for r >= 0 {
		header, err = c.chain.GetTrunkBlockHeader(r)

		if err != nil {
			r = r - 1
			continue
		}

		err = nil
		break
	}

	if err != nil {
		return thor.BytesToBytes32(nil), errors.New("Block not found")
	}

	for getRoundNumber(header, launchTime) > lastRound {
		header, err = c.chain.GetBlockHeader(header.ParentID())
		if err != nil {
			hex := hex.EncodeToString(header.ParentID().Bytes())
			return thor.BytesToBytes32(nil), errors.New("Block " + hex + " not found")
		}
	}

	return CompBeaconFromHeader(header), nil
}

// CompBeaconFromHeader computes the beacon from the given block header
func CompBeaconFromHeader(header *block.Header) thor.Bytes32 {
	return header.ID()
}

func getRoundNumber(header *block.Header, launchTime uint64) uint32 {
	return uint32((header.Timestamp() - launchTime) / thor.BlockInterval)
}

package consensus

import (
	"encoding/binary"
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
			continue
		}

		r = r - 1
	}

	for getRoundNumber(header, launchTime) > lastRound {
		header, err = c.chain.GetBlockHeader(header.ParentID())
		if err != nil {
			hex := hex.EncodeToString(header.ParentID().Bytes())
			return thor.BytesToBytes32(nil), errors.New("Failed to extract block with id = " + hex)
		}
	}

	return header.ParentID(), nil
}

func getRoundNumber(header *block.Header, launchTime uint64) uint32 {
	return uint32((header.Timestamp() - launchTime) / thor.BlockInterval)
}

// GetRoundSeed computes the random seed for each round
func GetRoundSeed(beacon thor.Bytes32, roundNum uint32) thor.Bytes32 {
	// round_seed = H(epoch_seed || round_number)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, roundNum)
	return thor.Blake2b(beacon.Bytes(), b)
}

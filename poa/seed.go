// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

var epochInterval uint32 = thor.EpochInterval

// mockEpochInterval mocks the epoch interval。
// TEST ONLY
func mockEpochInterval(interval uint32) {
	epochInterval = interval
}

// Seeder generates seed for poa scheduler.
type Seeder struct {
	repo  *chain.Repository
	cache map[thor.Bytes32]thor.Bytes32
}

// NewSeeder creates a seeder
func NewSeeder(repo *chain.Repository) *Seeder {
	return &Seeder{
		repo,
		make(map[thor.Bytes32]thor.Bytes32),
	}
}

// Generate creates a seed for the given parent block's header.
func (seeder *Seeder) Generate(parentID thor.Bytes32) (thor.Bytes32, error) {
	blockNum := block.Number(parentID) + 1

	epoch := blockNum / epochInterval
	if epoch <= 1 {
		return thor.Bytes32{}, nil
	}
	seedNum := (epoch - 1) * epochInterval

	blockID, err := seeder.repo.NewChain(parentID).GetBlockID(seedNum)
	if err != nil {
		return thor.Bytes32{}, err
	}

	seedSummary, err := seeder.repo.GetBlockSummary(blockID)

	if err != nil {
		return thor.Bytes32{}, err
	}

	if len(seedSummary.Beta()) == 0 {
		return thor.Bytes32{}, nil
	}

	if v, ok := seeder.cache[seedSummary.Header.ID()]; ok {
		return v, nil
	}

	hasher := thor.NewBlake2b()
	sum := seedSummary
	for i := 0; i < int(epochInterval); i++ {
		hasher.Write(sum.Beta())

		sum, err = seeder.repo.GetBlockSummary(sum.Header.ParentID())
		if err != nil {
			return thor.Bytes32{}, err
		}
		if len(sum.Beta()) == 0 {
			break
		}
	}

	var seed thor.Bytes32
	hasher.Sum(seed[:0])

	seeder.cache[seedSummary.Header.ID()] = seed
	return seed, nil
}

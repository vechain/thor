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
func (seeder *Seeder) Generate(parentID thor.Bytes32) (seed thor.Bytes32, err error) {
	blockNum := block.Number(parentID) + 1

	epoch := blockNum / epochInterval
	if epoch <= 1 {
		return
	}
	seedNum := (epoch - 1) * epochInterval

	seedBlk, err := seeder.repo.NewChain(parentID).GetBlockHeader(seedNum)
	if err != nil {
		return
	}

	if v, ok := seeder.cache[seedBlk.ID()]; ok {
		return v, nil
	}

	defer func() {
		if err != nil && !seed.IsZero() {
			seeder.cache[seedBlk.ID()] = seed
		}
	}()

	beta, err := seedBlk.Beta()
	if err != nil {
		return
	}

	if beta == nil {
		return thor.Bytes32{}, nil
	}

	return thor.Blake2b(beta), nil
}

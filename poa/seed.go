// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

var emptyRoot = thor.Blake2b(rlp.EmptyString) // This is the known root hash of an empty trie.

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

// Generate creates a seed for the given parent block's header. If the seed block contains at least one backer signature,
// concatenate the VRF outputs(beta) to create seed.
func (seeder *Seeder) Generate(parentID thor.Bytes32) (thor.Bytes32, error) {
	blockNum := block.Number(parentID) + 1

	epoch := blockNum / thor.EpochInterval
	if epoch <= 1 {
		return thor.Bytes32{}, nil
	}
	seedNum := (epoch - 1) * thor.EpochInterval

	seedBlock, err := seeder.repo.NewChain(parentID).GetBlockHeader(seedNum)
	if err != nil {
		return thor.Bytes32{}, err
	}

	// seedblock located at pre-VIP193 stage
	if len(seedBlock.Signature()) == 65 {
		return thor.Bytes32{}, nil
	}

	if v, ok := seeder.cache[seedBlock.ID()]; ok {
		return v, nil
	}

	hasher := thor.NewBlake2b()
	next := seedBlock.ID()
	for i := 0; i < thor.EpochInterval; i++ {
		sum, err := seeder.repo.GetBlockSummary(next)
		if err != nil {
			return thor.Bytes32{}, err
		}

		beta, err := sum.Header.Beta()
		if err != nil {
			return thor.Bytes32{}, err
		}

		if len(beta) == 0 {
			break
		}
		hasher.Write(beta)
		next = sum.Header.ParentID()
	}

	var seed thor.Bytes32
	hasher.Sum(seed[:0])

	seeder.cache[seedBlock.ID()] = seed
	return seed, nil
}

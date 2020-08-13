// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

var emptyRoot = thor.Blake2b(rlp.EmptyString) // This is the known root hash of an empty trie.

// Seeder generates seed for poa scheduler.
type Seeder struct {
	repo  *chain.Repository
	cache map[thor.Bytes32][]byte
}

// NewSeeder creates a seeder
func NewSeeder(repo *chain.Repository) *Seeder {
	return &Seeder{
		repo,
		make(map[thor.Bytes32][]byte),
	}
}

// Generate creates a seed for the given parent block's header. If the seed block contains at least one backer signature,
// concatenate the VRF outputs(beta) to create seed. Otherwise，returns nil.
func (seeder *Seeder) Generate(parentHeader *block.Header) ([]byte, error) {
	blockNum := parentHeader.Number() + 1

	round := blockNum / thor.EpochInterval
	if round < 1 {
		return nil, nil
	}
	seedNum := (round - 1) * thor.EpochInterval

	seedBlock, err := seeder.repo.NewChain(parentHeader.ID()).GetBlockHeader(seedNum)
	if err != nil {
		return nil, err
	}

	if v, ok := seeder.cache[seedBlock.ID()]; ok == true {
		return v, nil
	}
	var seed []byte
	if seedBlock.BackerSignaturesRoot() != emptyRoot {
		bss, err := seeder.repo.GetBlockBackerSignatures(seedBlock.ID())
		if err != nil {
			return nil, err
		}

		signer, err := seedBlock.Signer()
		if err != nil {
			return nil, err
		}

		alpha := seedBlock.Proposal().Alpha(signer)

		for _, bs := range bss {
			beta, err := bs.Validate(alpha[:])
			if err != nil {
				return nil, err
			}
			seed = append(seed, beta...)
		}
	} else {
		signer, err := seedBlock.Signer()
		if err != nil {
			return nil, err
		}

		t := make([]byte, 8)
		binary.BigEndian.PutUint64(t, seedBlock.TotalBackersCount())

		seed = append(seed, signer.Bytes()...)
		seed = append(seed, t...)
	}

	seeder.cache[seedBlock.ID()] = seed
	return seed, nil
}

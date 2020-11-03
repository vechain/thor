// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/go-ecvrf"
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
	if epoch < 1 {
		return thor.Bytes32{}, nil
	}
	seedNum := (epoch - 1) * thor.EpochInterval

	seedBlock, err := seeder.repo.NewChain(parentID).GetBlockHeader(seedNum)
	if err != nil {
		return thor.Bytes32{}, err
	}

	if v, ok := seeder.cache[seedBlock.ID()]; ok == true {
		return v, nil
	}

	signer, err := seedBlock.Signer()
	if err != nil {
		return thor.Bytes32{}, err
	}

	hasher := thor.NewBlake2b()
	hasher.Write(signer.Bytes())

	t := make([]byte, 8)
	binary.BigEndian.PutUint64(t, seedBlock.TotalBackersCount())
	hasher.Write(t)

	if seedBlock.BackerSignaturesRoot() != emptyRoot {
		// the seed corresponding to the seed block
		theSeed, err := seeder.Generate(seedBlock.ParentID())
		if err != nil {
			return thor.Bytes32{}, err
		}

		alpha := append([]byte(nil), theSeed.Bytes()...)
		alpha = append(alpha, seedBlock.ParentID().Bytes()[:4]...)

		hash := block.NewProposal(seedBlock.ParentID(), seedBlock.TxsRoot(), seedBlock.GasLimit(), seedBlock.Timestamp()).Hash()
		bss, err := seeder.repo.GetBlockBackerSignatures(seedBlock.ID())
		if err != nil {
			return thor.Bytes32{}, err
		}
		for _, bs := range bss {
			pub, err := crypto.SigToPub(hash.Bytes(), bs.Signature())
			if err != nil {
				return thor.Bytes32{}, err
			}
			beta, err := ecvrf.NewSecp256k1Sha256Tai().Verify(pub, alpha, bs.Proof())
			if err != nil {
				return thor.Bytes32{}, err
			}
			hasher.Write(beta)
		}
	}

	var seed thor.Bytes32
	hasher.Sum(seed[:])

	seeder.cache[seedBlock.ID()] = seed
	return seed, nil
}

// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"bytes"
	"sort"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

var emptyRoot = thor.Blake2b(rlp.EmptyString) // This is the known root hash of an empty trie.

// Seeder generates seed for poa scheduler.
type Seeder struct {
	repo       *chain.Repository
	forkConfig thor.ForkConfig
}

// NewSeeder creates a seeder
func NewSeeder(repo *chain.Repository, forkConfig thor.ForkConfig) *Seeder {
	return &Seeder{
		repo,
		forkConfig,
	}
}

// Generate creates a seed for the given parent block's header. Seeder will traverse back by parentID.
// Until there is a block contains at least one backer signature, concatenate the VRF outputs(beta) to create seed.
func (seeder *Seeder) Generate(parentHeader *block.Header) ([]byte, error) {
	if parentHeader.Number() <= 1 || parentHeader.Number() <= seeder.forkConfig.VIP193 {
		return nil, nil
	}

	b := parentHeader
	for {
		summary, err := seeder.repo.GetBlockSummary(b.ParentID())
		if err != nil {
			return nil, nil
		}
		b = summary.Header

		if b.Number() < seeder.forkConfig.VIP193 || b.Number() == 0 {
			return nil, nil
		}

		if b.BackerSignaturesRoot() != emptyRoot {
			bss, err := seeder.repo.GetBlockBackerSignatures(b.ID())
			if err != nil {
				return nil, err
			}

			signer, err := b.Signer()
			if err != nil {
				return nil, err
			}

			alpha := b.Proposal().Alpha(signer)
			betas := make([][]byte, len(bss))

			for _, bs := range bss {
				beta, err := bs.Validate(alpha[:])
				if err != nil {
					return nil, err
				}
				betas = append(betas, beta)
			}
			sort.Slice(betas, func(i, j int) bool {
				return bytes.Compare(betas[i], betas[j]) < 0
			})

			var seed []byte
			for _, b := range betas {
				seed = append(seed, b...)
			}

			return seed, nil
		}
	}
}

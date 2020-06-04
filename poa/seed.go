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
// Until there is a block contains backer's approval, takes the VRF output to create seed.
func (seeder *Seeder) Generate(parentHeader *block.Header) (thor.Bytes32, error) {
	if parentHeader.Number() < 2 || parentHeader.Number() < seeder.forkConfig.VIP193+1 {
		return thor.Bytes32{}, nil
	}

	b := parentHeader
	for {
		summary, err := seeder.repo.GetBlockSummary(b.ParentID())
		if err != nil {
			return thor.Bytes32{}, err
		}
		b = summary.Header

		if b.Number() < seeder.forkConfig.VIP193 || b.Number() == 0 {
			return thor.Bytes32{}, nil
		}

		if b.BackersRoot() != emptyRoot {
			backers, err := seeder.repo.GetBlockBackers(b.ID())
			if err != nil {
				return thor.Bytes32{}, err
			}

			hasher := thor.NewBlake2b()

			alpha := b.Proposal().Hash().Bytes()
			for _, approval := range backers {
				beta, err := approval.Validate(alpha)
				if err != nil {
					return thor.Bytes32{}, err
				}
				hasher.Write(beta)
			}

			var hash thor.Bytes32
			hasher.Sum(hash[:0])
			return hash, nil
		}
	}
}

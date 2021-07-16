// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package block

import (
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/go-ecvrf"
	"github.com/vechain/thor/thor"
)

type _cache struct {
	backers []thor.Address
	betas   [][]byte
}

// Committee is the set of block backer signatures.
type Committee struct {
	proposalHash thor.Bytes32
	alpha        []byte
	bss          ComplexSignatures
	cache        atomic.Value
}

// NewCommittee creates a new committee.
func NewCommittee(proposalHash thor.Bytes32, alpha []byte, bss ComplexSignatures) *Committee {
	return &Committee{
		proposalHash: proposalHash,
		alpha:        alpha,
		bss:          bss,
	}
}

// Members returns the committee member's address along with their VRF variable: beta.
func (cmt *Committee) Members() ([]thor.Address, [][]byte, error) {
	if cached := cmt.cache.Load(); cached != nil {
		return cached.(_cache).backers, cached.(_cache).betas, nil
	}

	if (len(cmt.bss)) > 0 {
		var c _cache
		c.backers = make([]thor.Address, 0, len(cmt.bss))
		c.betas = make([][]byte, 0, len(cmt.bss))

		for _, bs := range cmt.bss {
			pub, err := crypto.SigToPub(cmt.proposalHash.Bytes(), bs.Signature())
			if err != nil {
				return nil, nil, err
			}
			c.backers = append(c.backers, thor.Address(crypto.PubkeyToAddress(*pub)))

			beta, err := ecvrf.NewSecp256k1Sha256Tai().Verify(pub, cmt.alpha, bs.Proof())
			if err != nil {
				return nil, nil, err
			}
			c.betas = append(c.betas, beta)
		}

		cmt.cache.Store(c)
		return c.backers, c.betas, nil
	}

	return nil, nil, nil
}

// BackerSignatures returns the signatures.
func (cmt *Committee) BackerSignatures() ComplexSignatures {
	return append(ComplexSignatures(nil), cmt.bss...)
}

// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"errors"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

var errConflictWithFinalized = errors.New("block conflict with finalized")

func IsConflictWithFinalized(err error) bool {
	return err == errConflictWithFinalized
}

type voted map[thor.Bytes32]uint32
type votes []struct {
	checkpoint thor.Bytes32
	quality    uint32
}

func newVoted(engine *BFTEngine) (voted, error) {
	v := make(voted)

	finalized := engine.Finalized()
	heads, err := engine.repo.ScanHeads(block.Number(finalized))
	if err != nil {
		return nil, err
	}

	for _, head := range heads {
		chain := engine.repo.NewChain(head)

		cur := head
		for {
			sum, err := engine.repo.GetBlockSummary(cur)
			if err != nil {
				return nil, err
			}

			header := sum.Header
			signer, _ := header.Signer()
			if signer == engine.master {
				st, err := engine.computeState(header)
				if err != nil {
					return nil, err
				}
				// jump to previous round
				sum, err = chain.GetBlockSummary(getCheckPoint(header.Number()))
				if err != nil {
					return nil, err
				}

				checkpoint := sum.Header.ID()
				if quality, ok := v[checkpoint]; !ok || quality < st.Quality {
					v[checkpoint] = st.Quality
				}
			}

			if sum.Header.Number() == block.Number(finalized) {
				break
			}
			cur = sum.Header.ParentID()
		}
	}

	return v, nil
}

func (v voted) Votes(finalized thor.Bytes32) votes {
	list := make(votes, 0, len(v))

	for checkpoint, quality := range v {
		if block.Number(checkpoint) >= block.Number(finalized) {
			list = append(list, struct {
				checkpoint thor.Bytes32
				quality    uint32
			}{checkpoint: checkpoint, quality: quality})
		}
	}
	return list
}

func (v voted) Vote(checkpoint thor.Bytes32, quality uint32) {
	v[checkpoint] = quality
}

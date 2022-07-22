// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"bytes"
	"sort"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// casts stores the master's overall casts, maintaining the map of quality to checkpoint.
type casts map[thor.Bytes32]uint32

func (engine *BFTEngine) newCasts() error {
	c := make(casts)

	finalized := engine.Finalized()
	heads, err := engine.repo.ScanHeads(block.Number(finalized))
	if err != nil {
		return err
	}

	for _, head := range heads {
		chain := engine.repo.NewChain(head)

		cur := head
		for {
			sum, err := engine.repo.GetBlockSummary(cur)
			if err != nil {
				return err
			}

			header := sum.Header
			signer, _ := header.Signer()
			if signer == engine.master {
				st, err := engine.computeState(header)
				if err != nil {
					return err
				}

				checkpoint, err := chain.GetBlockID(getCheckPoint(header.Number()))
				if err != nil {
					return err
				}
				if quality, ok := c[checkpoint]; !ok || quality < st.Quality {
					c[checkpoint] = st.Quality
				}
				break
			}

			if sum.Header.Number() <= block.Number(finalized) {
				break
			}
			cur = sum.Header.ParentID()
		}
	}

	engine.casts = c
	return nil
}

// Slice dumps the casts that is after finalized into slice.
func (ca casts) Slice(finalized thor.Bytes32) []struct {
	checkpoint thor.Bytes32
	quality    uint32
} {
	list := make([]struct {
		checkpoint thor.Bytes32
		quality    uint32
	}, 0, len(ca))

	for checkpoint, quality := range ca {
		if block.Number(checkpoint) >= block.Number(finalized) {
			list = append(list, struct {
				checkpoint thor.Bytes32
				quality    uint32
			}{checkpoint: checkpoint, quality: quality})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return bytes.Compare(list[i].checkpoint.Bytes(), list[j].checkpoint.Bytes()) > 0
	})

	return list
}

// Mark marks the master's cast.
func (ca casts) Mark(checkpoint thor.Bytes32, quality uint32) {
	ca[checkpoint] = quality
}

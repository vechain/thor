// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

// ExtendedBlock extend block.Block with the obsolete flag.
type ExtendedBlock struct {
	*block.Block
	Obsolete bool
}

// BlockReader defines the interface to stream block activity.
type BlockReader interface {
	Read() ([]*ExtendedBlock, error)
}

type readBlockFunc func() ([]*ExtendedBlock, error)

func (r readBlockFunc) Read() ([]*ExtendedBlock, error) {
	return r()
}

// NewBlockReader create BlockReader instance.
func (r *Repository) NewBlockReader(position thor.Bytes32) BlockReader {
	return readBlockFunc(func() ([]*ExtendedBlock, error) {
		bestChain := r.NewBestChain()
		if bestChain.HeadID() == position {
			return nil, nil
		}

		headNum := block.Number(bestChain.HeadID())

		var blocks []*ExtendedBlock
		for {
			cur, err := r.GetBlock(position)
			if err != nil {
				return nil, err
			}

			if block.Number(position) > headNum {
				blocks = append(blocks, &ExtendedBlock{cur, true})
				position = cur.Header().ParentID()
				continue
			}

			has, err := bestChain.HasBlock(position)
			if err != nil {
				return nil, err
			}

			if has {
				next, err := bestChain.GetBlock(block.Number(position) + 1)
				if err != nil {
					return nil, err
				}

				position = next.Header().ID()
				return append(blocks, &ExtendedBlock{next, false}), nil
			}

			blocks = append(blocks, &ExtendedBlock{cur, true})
			position = cur.Header().ParentID()
		}
	})
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
)

func TestBlockReader(t *testing.T) {
	_, repo := newTestRepo()
	b0 := repo.GenesisBlock()

	b1 := newBlock(b0, 10)
	repo.AddBlock(b1, nil, 0, false)

	b2 := newBlock(b1, 20)
	repo.AddBlock(b2, nil, 0, false)

	b3 := newBlock(b2, 30)
	repo.AddBlock(b3, nil, 0, false)

	b4 := newBlock(b3, 40)
	repo.AddBlock(b4, nil, 0, true)

	br := repo.NewBlockReader(b2.Header().ID())

	var blks []*ExtendedBlock

	for {
		r, err := br.Read()
		if err != nil {
			panic(err)
		}
		if len(r) == 0 {
			break
		}
		blks = append(blks, r...)
	}

	assert.Equal(t, []*ExtendedBlock{
		{block.Compose(b3.Header(), b3.Transactions()), false},
		{block.Compose(b4.Header(), b4.Transactions()), false}},
		blks)
}

func TestBlockReaderFork(t *testing.T) {
	_, repo := newTestRepo()
	b0 := repo.GenesisBlock()

	b1 := newBlock(b0, 10)
	repo.AddBlock(b1, nil, 0, false)

	b2 := newBlock(b1, 20)
	repo.AddBlock(b2, nil, 0, false)

	b2x := newBlock(b1, 20)
	repo.AddBlock(b2x, nil, 1, false)

	b3 := newBlock(b2, 30)
	repo.AddBlock(b3, nil, 0, false)

	b4 := newBlock(b3, 40)
	repo.AddBlock(b4, nil, 0, true)

	br := repo.NewBlockReader(b2x.Header().ID())

	var blks []*ExtendedBlock

	for {
		r, err := br.Read()
		if err != nil {
			panic(err)
		}
		if len(r) == 0 {
			break
		}

		blks = append(blks, r...)
	}

	assert.Equal(t, []*ExtendedBlock{
		{block.Compose(b2x.Header(), b2x.Transactions()), true},
		{block.Compose(b2.Header(), b2.Transactions()), false},
		{block.Compose(b3.Header(), b3.Transactions()), false},
		{block.Compose(b4.Header(), b4.Transactions()), false}},
		blks)
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockReader(t *testing.T) {
	repo := initRepo()
	b0 := repo.GenesisBlock()

	b1 := newBlock(b0, 2)
	repo.AddBlock(b1, nil)

	b2 := newBlock(b1, 2)
	repo.AddBlock(b2, nil)

	b3 := newBlock(b2, 2)
	repo.AddBlock(b3, nil)

	b4 := newBlock(b3, 2)
	repo.AddBlock(b4, nil)

	repo.SetBestBlockID(b4.Header().ID())

	br := repo.NewBlockReader(b2.Header().ID())

	blks, err := br.Read()
	assert.Nil(t, err)
	assert.Equal(t, blks[0].Header().ID(), b3.Header().ID())
	assert.False(t, blks[0].Obsolete)

	blks, err = br.Read()
	assert.Nil(t, err)
	assert.Equal(t, blks[0].Header().ID(), b4.Header().ID())
	assert.False(t, blks[0].Obsolete)
}

func TestBlockReaderFork(t *testing.T) {
	repo := initRepo()
	b0 := repo.GenesisBlock()

	b1 := newBlock(b0, 2)
	repo.AddBlock(b1, nil)

	b2 := newBlock(b1, 2)
	repo.AddBlock(b2, nil)

	b2x := newBlock(b1, 1)
	repo.AddBlock(b2x, nil)

	b3 := newBlock(b2, 2)
	repo.AddBlock(b3, nil)

	b4 := newBlock(b3, 2)
	repo.AddBlock(b4, nil)

	repo.SetBestBlockID(b4.Header().ID())

	br := repo.NewBlockReader(b2x.Header().ID())

	blks, err := br.Read()
	assert.Nil(t, err)
	assert.Equal(t, blks[0].Header().ID(), b2x.Header().ID())
	assert.True(t, blks[0].Obsolete)
	assert.Equal(t, blks[1].Header().ID(), b2.Header().ID())
	assert.False(t, blks[1].Obsolete)

	blks, err = br.Read()
	assert.Nil(t, err)
	assert.Equal(t, blks[0].Header().ID(), b3.Header().ID())
	assert.False(t, blks[0].Obsolete)
}

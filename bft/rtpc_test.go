package bft

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/thor"
)

func TestUpdate(t *testing.T) {
	//								|-v4-|
	// 				    	   b7 <-- b8
	//				   		  /			  |--v3--|
	// b0 <-- b1 <-- b2 <-- b3 <-- b4 <-- b5 <-- b6
	//        |--v1--|       |--v2--|      \
	//							            \
	//										 b10
	// 										|-v5-|

	// v1: b1-b2, 	npp(b0) = 2f+1
	// v2: b3-b4, 	npp(b1) = 2f+1, 	npc(b0) = 0
	// v3: b5-b6, 		        		npc(b1) > 0, npc(b7) > 0
	// v4: b8, 		npp(b7) = 2f+1, 	npc(b1) = 0
	// v5: b10, 						npc(b7) = 0
	//
	// v1 < v2 < v4 < v3 < v5
	//
	// blocks arriving order: b0, b1, b2, b3, b4, b7, b5, b6, b8, b10

	emptyBytes32 := thor.Bytes32{}

	repo := newTestRepo()
	gen := repo.GenesisBlock()
	rtpc := newRTPC(repo, gen.Header().ID())

	b0 := newBlock(0, nil, gen, 1, [4]thor.Bytes32{})
	assert.Nil(t, repo.AddBlock(b0, nil))

	// Add v1: b1-b2, rtpc: nil -> b0
	b1 := newBlock(
		0, inds[1:30], b0, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(b0.Header().Number() + 1), b0.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b1, nil))
	rtpc.update(b1)
	assert.Nil(t, rtpc.get())

	b2 := newBlock(
		30, inds[31:68], b1, 1,
		[4]thor.Bytes32{b1.Header().ID(), b0.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b2, nil))
	rtpc.update(b2)
	assert.Equal(t, b0.Header().ID(), rtpc.get().ID())

	// Add v2: b3-b4, rtpc: b0 -> b1
	b3 := newBlock(
		0, inds[1:30], b2, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(b2.Header().Number() + 1), b1.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b3, nil))
	rtpc.update(b3)
	assert.Equal(t, b0.Header().ID(), rtpc.get().ID())

	b4 := newBlock(
		30, inds[31:68], b3, 1,
		[4]thor.Bytes32{b3.Header().ID(), b1.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b4, nil))
	rtpc.update(b4)
	assert.Equal(t, b1.Header().ID(), rtpc.get().ID())

	// Add b7, rtpc: b1 -> b1
	b7 := newBlock(0, nil, b3, 1, [4]thor.Bytes32{})
	assert.Nil(t, repo.AddBlock(b7, nil))
	rtpc.update(b7)
	assert.Equal(t, b1.Header().ID(), rtpc.get().ID())

	// Add v3: b5-b6, rtpc: b1 -> b1
	b5 := newBlock(
		0, inds[1:30], b4, 3,
		[4]thor.Bytes32{
			GenNVForFirstBlock(b4.Header().Number() + 1),
			emptyBytes32, b7.Header().ID(),
		},
	)
	assert.Nil(t, repo.AddBlock(b5, nil))
	rtpc.update(b5)
	assert.Equal(t, b1.Header().ID(), rtpc.get().ID())

	b6 := newBlock(
		30, inds[31:68], b5, 1,
		[4]thor.Bytes32{b5.Header().ID(), emptyBytes32, b1.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b6, nil))
	rtpc.update(b6)
	assert.Equal(t, b1.Header().ID(), rtpc.get().ID())

	// Add v4: b8, rtpc: b1 -> b7
	b8 := newBlock(
		0, inds[:68], b7, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(b4.Header().Number() + 1), b7.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b8, nil))
	rtpc.update(b8)
	assert.Equal(t, b7.Header().ID(), rtpc.get().ID())

	// Add v5: b10, rtpc: b1 -> nil
	b10 := newBlock(
		0, inds[:68], b5, 1,
		[4]thor.Bytes32{GenNVForFirstBlock(b5.Header().Number() + 1)},
	)
	assert.Nil(t, repo.AddBlock(b10, nil))
	rtpc.update(b10)
	assert.Nil(t, rtpc.get())
}

func TestUpdateLastCommitted(t *testing.T) {
	proposer := rand.Intn(nNode)

	repo := newTestRepo()
	gen := repo.GenesisBlock()
	rtpc := newRTPC(repo, gen.Header().ID())

	b1 := newBlock(proposer, nil, gen, 1, [4]thor.Bytes32{})
	b2 := newBlock(proposer, nil, b1, 1, [4]thor.Bytes32{})
	b3 := newBlock(proposer, nil, b2, 1, [4]thor.Bytes32{})
	b4 := newBlock(proposer, nil, b1, 1, [4]thor.Bytes32{})
	repo.AddBlock(b1, nil)
	repo.AddBlock(b2, nil)
	repo.AddBlock(b3, nil)
	repo.AddBlock(b4, nil)

	rtpc.curr = b2.Header()

	err := rtpc.updateLastCommitted(b1.Header().ID())
	assert.Nil(t, err)
	assert.Equal(t, b2.Header().ID(), rtpc.get().ID())

	err = rtpc.updateLastCommitted(b2.Header().ID())
	assert.Nil(t, err)
	assert.Nil(t, rtpc.get())

	err = rtpc.updateLastCommitted(b3.Header().ID())
	assert.Nil(t, err)
	assert.Nil(t, rtpc.get())

	err = rtpc.updateLastCommitted(b4.Header().ID())
	assert.Equal(t, errors.New("Input block must be an offspring of the last committed"), err)
}

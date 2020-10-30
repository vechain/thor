package bft

import (
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/thor"
)

func TestUpdate(t *testing.T) {
	// 				    	   b7 <-- b8
	//				   		  /
	// b0 <-- b1 <-- b2 <-- b3 <-- b4 <-- b5 <-- b6
	//							            \
	//										 b10

	// v1: b1-b2, 	pp(b0) = 2f+1
	// v2: b3-b4, 	pp(b1) = 2f+1, 	npc(b0) = 0
	// v3: b5-b6, 		        	npc(b1) > 0, npc(b7) > 0
	// v4: b8, 		pp(b7) = 2f+1, 	npc(b1) = 0
	// v5: b10, 					npc(b7) = 0
	//
	// v1 < v2 < v4 < v3 < v5
	//
	// blocks arriving order: b0, b1, b2, b3, b4, b7, b5, b6, b8, b10

	// Generate private keys for nodes
	keys := []*ecdsa.PrivateKey(nil)
	for i := 0; i < nNode; i++ {
		key, _ := crypto.GenerateKey()
		keys = append(keys, key)
	}

	repo := newTestRepo()
	rtpc := newRTPC(repo)
	gen := repo.GenesisBlock()

	b0 := newBlock(keys[0], nil, gen.Header().ID(), gen.Header().Timestamp()+10, 0, [4]thor.Bytes32{})
	assert.Nil(t, repo.AddBlock(b0, nil))

	// Add v1: b1-b2, rtpc: nil -> b0
	b1 := newBlock(
		keys[0], keys[1:30],
		b0.Header().ID(), b0.Header().Timestamp()+10, 0,
		[4]thor.Bytes32{GenNVforFirstBlock(b0.Header().Number() + 1), b0.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b1, nil))
	rtpc.updateByNewBlock(b1)
	assert.Nil(t, rtpc.getRTPC())

	b2 := newBlock(
		keys[30], keys[31:68],
		b1.Header().ID(), b1.Header().Timestamp()+10, 0,
		[4]thor.Bytes32{b1.Header().ID(), b0.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b2, nil))
	rtpc.updateByNewBlock(b2)
	assert.Equal(t, b0.Header().ID(), rtpc.getRTPC().ID())

	// Add v2: b3-b4, rtpc: b0 -> b1
	b3 := newBlock(
		keys[0], keys[1:30],
		b2.Header().ID(), b2.Header().Timestamp()+10, 0,
		[4]thor.Bytes32{GenNVforFirstBlock(b2.Header().Number() + 1), b1.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b3, nil))
	rtpc.updateByNewBlock(b3)
	assert.Equal(t, b0.Header().ID(), rtpc.getRTPC().ID())

	b4 := newBlock(
		keys[30], keys[31:68],
		b3.Header().ID(), b3.Header().Timestamp()+10, 0,
		[4]thor.Bytes32{b3.Header().ID(), b1.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b4, nil))
	rtpc.updateByNewBlock(b4)
	assert.Equal(t, b1.Header().ID(), rtpc.getRTPC().ID())

	// Add b7, rtpc: b1 -> b1
	b7 := newBlock(keys[0], nil, b3.Header().ID(), b3.Header().Timestamp()+10, 0, [4]thor.Bytes32{})
	assert.Nil(t, repo.AddBlock(b7, nil))
	rtpc.updateByNewBlock(b7)
	assert.Equal(t, b1.Header().ID(), rtpc.getRTPC().ID())

	// Add v3: b5-b6, rtpc: b1 -> b1
	b5 := newBlock(
		keys[0], keys[1:30],
		b4.Header().ID(), b4.Header().Timestamp()+20, 0,
		[4]thor.Bytes32{
			GenNVforFirstBlock(b4.Header().Number() + 1),
			thor.Bytes32{}, b3.Header().ID(),
		},
	)
	assert.Nil(t, repo.AddBlock(b5, nil))
	rtpc.updateByNewBlock(b5)
	assert.Equal(t, b1.Header().ID(), rtpc.getRTPC().ID())

	b6 := newBlock(
		keys[30], keys[31:68],
		b5.Header().ID(), b5.Header().Timestamp()+10, 0,
		[4]thor.Bytes32{b5.Header().ID(), thor.Bytes32{}, b1.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b6, nil))
	rtpc.updateByNewBlock(b6)
	assert.Equal(t, b1.Header().ID(), rtpc.getRTPC().ID())

	// Add v4: b8, rtpc: b1 -> b7
	b8 := newBlock(
		keys[0], keys[:68],
		b7.Header().ID(), b7.Header().Timestamp()+10, 0,
		[4]thor.Bytes32{GenNVforFirstBlock(b4.Header().Number() + 1), b7.Header().ID()},
	)
	assert.Nil(t, repo.AddBlock(b8, nil))
	rtpc.updateByNewBlock(b8)
	assert.Equal(t, b7.Header().ID(), rtpc.getRTPC().ID())

	// Add v5: b10, rtpc: b1 -> nil
	b10 := newBlock(
		keys[0], keys[:68],
		b5.Header().ID(), b5.Header().Timestamp()+10, 0,
		[4]thor.Bytes32{GenNVforFirstBlock(b5.Header().Number() + 1)},
	)
	assert.Nil(t, repo.AddBlock(b10, nil))
	rtpc.updateByNewBlock(b10)
	assert.Nil(t, rtpc.getRTPC())
}

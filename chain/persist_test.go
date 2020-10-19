package chain

import (
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
)

func randBlockID(num uint32) (b thor.Bytes32) {
	rand.Read(b[:])
	binary.BigEndian.PutUint32(b[:], num)
	return
}

func TestSaveBranchHead(t *testing.T) {
	db := muxdb.NewMem()
	s := db.NewStore("test")

	// Test saveBranchHead
	id1 := randBlockID(uint32(1))
	id2 := randBlockID(uint32(2))

	saveBranchHead(s, id1, thor.Bytes32{})
	assert.True(t, isBranchHead(s, id1))

	saveBranchHead(s, id2, id1)
	assert.True(t, isBranchHead(s, id2))
	assert.False(t, isBranchHead(s, id1))
}

func TestLoadBranchHeads(t *testing.T) {
	db := muxdb.NewMem()
	s := db.NewStore("test")

	for i := 0; i < 10; i++ {
		id := randBlockID(uint32(i))
		saveBranchHead(s, id, thor.Bytes32{})
		assert.True(t, isBranchHead(s, id))
	}

	minNum := uint32(5)
	heads := loadBranchHeads(s, minNum)

	assert.Equal(t, len(heads), 5)

	for _, head := range heads {
		assert.True(t, block.Number(head) >= minNum)
	}
}

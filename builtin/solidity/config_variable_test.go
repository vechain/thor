package solidity

import (
	"encoding/binary"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func TestConfigVariable(t *testing.T) {
	config := NewConfigVariable("name", 10)

	value := config.Get()
	assert.Equal(t, uint32(10), value)

	name := config.Name()
	assert.Equal(t, "name", name)

	slot := config.Slot()
	assert.Equal(t, thor.BytesToBytes32([]byte("name")), slot)

	ctx := newContext()
	config.Override(ctx)

	config.initialised = true
	config.Override(ctx)

	config.initialised = false
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("cfg"))
	ctx = NewContext(addr, st, nil)

	config = NewConfigVariable("test", 10)
	st.SetRawStorage(addr, config.Slot(), rlp.RawValue{0xFF})
	config.Override(ctx)
	value = config.Get()
	assert.Equal(t, uint32(10), value)

	var be8 [8]byte
	binary.BigEndian.PutUint64(be8[:], 1<<40)
	st.SetStorage(addr, config.Slot(), thor.BytesToBytes32(be8[:]))

	config.Override(ctx)
	value = config.Get()
	assert.Equal(t, uint32(0), value)
}

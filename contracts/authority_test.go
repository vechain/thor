package contracts

import (
	"testing"

	"github.com/vechain/thor/poa"

	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"

	"github.com/stretchr/testify/assert"
)

func TestAuthority(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	assert.True(t, len(Authority.RuntimeBytecodes()) > 0)

	p1 := thor.BytesToAddress([]byte("p1"))
	p2 := thor.BytesToAddress([]byte("p2"))
	p3 := thor.BytesToAddress([]byte("p3"))

	assert.True(t, Authority.Add(st, p1))
	assert.True(t, Authority.Add(st, p2))
	assert.True(t, Authority.Add(st, p3))

	assert.False(t, Authority.Add(st, p1), "add duped proposer should fail")

	assert.Equal(t, uint64(3), Authority.Count(st))

	assert.Equal(t, []poa.Proposer{{p1, 0}, {p2, 0}, {p3, 0}}, Authority.All(st))

	listed, status := Authority.Status(st, p1)
	assert.True(t, listed)
	assert.Equal(t, uint32(0), status)

	assert.True(t, Authority.Update(st, p1, 1))
	_, status = Authority.Status(st, p1)
	assert.Equal(t, uint32(1), status)

	assert.True(t, Authority.Remove(st, p1))
	assert.Equal(t, []poa.Proposer{{p3, 0}, {p2, 0}}, Authority.All(st))
}

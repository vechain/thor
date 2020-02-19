package poa

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/builtin/authority"
	"github.com/vechain/thor/thor"
)

func randBytes32() thor.Bytes32 {
	var b thor.Bytes32
	rand.Read(b[:])
	return b
}

func randAddress() thor.Address {
	var b thor.Address
	rand.Read(b[:])
	return b
}

func randCandidate() *authority.Candidate {
	return &authority.Candidate{
		NodeMaster:   randAddress(),
		Endorsor:     randAddress(),
		Identity:     randBytes32(),
		Active:       true,
		VrfPublicKey: randBytes32(),
	}
}

func TestCandidatesCopy(t *testing.T) {
	c := new(Candidates)

	c.masters = make(map[thor.Address]int)
	c.endorsors = make(map[thor.Address]bool)

	for i := 0; i < 10; i++ {
		candidate := randCandidate()
		c.list = append(c.list, candidate)
		c.masters[candidate.NodeMaster] = i
		c.endorsors[candidate.NodeMaster] = i%2 == 0
	}

	cpy := c.Copy()
	assert.NotEqual(t, fmt.Sprintf("%p", c), fmt.Sprintf("%p", cpy))
	assert.NotEqual(t, fmt.Sprintf("%p", c.masters), fmt.Sprintf("%p", cpy.masters))
	assert.NotEqual(t, fmt.Sprintf("%p", c.endorsors), fmt.Sprintf("%p", cpy.endorsors))

	for i := 0; i < 10; i++ {
		assert.NotEqual(t, fmt.Sprintf("%p", c.list[i]), fmt.Sprintf("%p", cpy.list[i]))

		assert.Equal(t,
			[]interface{}{c.list[i].NodeMaster, c.list[i].Endorsor, c.list[i].Identity, c.list[i].VrfPublicKey},
			[]interface{}{cpy.list[i].NodeMaster, cpy.list[i].Endorsor, cpy.list[i].Identity, cpy.list[i].VrfPublicKey},
		)

		assert.Equal(t, c.masters[c.list[i].NodeMaster], cpy.masters[c.list[i].NodeMaster])

		assert.Equal(t, c.endorsors[c.list[i].NodeMaster], cpy.endorsors[c.list[i].NodeMaster])
	}
}

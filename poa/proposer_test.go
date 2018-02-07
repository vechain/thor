package poa_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/poa"
)

func TestProposer(t *testing.T) {
	p := poa.Proposer{}
	assert.False(t, p.IsOnline())
	p.SetOnline(false)
	assert.False(t, p.IsOnline())
	p.SetOnline(true)
	assert.True(t, p.IsOnline())

	var q poa.Proposer
	q.Decode(p.Encode())
	assert.Equal(t, p, q)
}

package schedule_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/schedule"
)

func TestProposer(t *testing.T) {
	p := schedule.Proposer{}
	assert.False(t, p.IsAbsent())
	p.SetAbsent(true)
	assert.True(t, p.IsAbsent())
	p.SetAbsent(false)
	assert.False(t, p.IsAbsent())

	var q schedule.Proposer
	q.Decode(p.Encode())
	assert.Equal(t, p, q)
}

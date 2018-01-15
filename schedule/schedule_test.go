package schedule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/thor"
)

func TestProposer(t *testing.T) {
	p := Proposer{}
	assert.False(t, p.isAbsent())
	p.setAbsent(true)
	assert.True(t, p.isAbsent())
	p.setAbsent(false)
	assert.False(t, p.isAbsent())
}

func TestSchedule(t *testing.T) {
	assert := assert.New(t)
	_ = assert
	p1, p2, p3, p4, p5 :=
		thor.BytesToAddress([]byte("p1")),
		thor.BytesToAddress([]byte("p2")),
		thor.BytesToAddress([]byte("p3")),
		thor.BytesToAddress([]byte("p4")),
		thor.BytesToAddress([]byte("p5"))

	proposers := []Proposer{
		{p1, 0},
		{p2, 0},
		{p3, 0},
		{p4, 0},
		{p5, 0},
	}
	_ = proposers

	parentTime := uint64(1000)
	sched := New(proposers, 1, parentTime)

	for i := uint64(0); i < 100; i++ {
		now := parentTime + i*thor.BlockInterval/2
		for _, p := range proposers {
			ts, _, _ := sched.Timing(p.Address, now)
			r, _, _ := sched.Validate(p.Address, ts)
			assert.True(r)
		}
	}
}

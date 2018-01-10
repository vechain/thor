package schedule_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/consensus/schedule"
	"github.com/vechain/thor/thor"
)

func TestSchedule(t *testing.T) {
	assert := assert.New(t)
	_ = assert
	p1, p2, p3, p4, p5 :=
		thor.BytesToAddress([]byte("p1")),
		thor.BytesToAddress([]byte("p2")),
		thor.BytesToAddress([]byte("p3")),
		thor.BytesToAddress([]byte("p4")),
		thor.BytesToAddress([]byte("p5"))

	proposers := []thor.Address{p1, p2, p3, p4, p5}
	_ = proposers

	parentTime := uint64(1000)
	sched := schedule.New(proposers, nil, 1, parentTime)

	for i := uint64(0); i < 100; i++ {
		now := parentTime + i*thor.BlockInterval/2
		for _, p := range proposers {
			ts, _, _ := sched.Timing(p, now)
			r, _ := sched.Validate(p, ts)
			assert.True(r)
		}
	}
}

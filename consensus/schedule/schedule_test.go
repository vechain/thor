package schedule_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/consensus/schedule"
	"github.com/vechain/thor/params"
)

func TestSchedule(t *testing.T) {
	assert := assert.New(t)
	_ = assert
	p1, p2, p3, p4, p5 :=
		acc.BytesToAddress([]byte("p1")),
		acc.BytesToAddress([]byte("p2")),
		acc.BytesToAddress([]byte("p3")),
		acc.BytesToAddress([]byte("p4")),
		acc.BytesToAddress([]byte("p5"))

	proposers := []acc.Address{p1, p2, p3, p4, p5}
	_ = proposers

	parentTime := uint64(1000)
	sched := schedule.New(proposers, nil, 1, parentTime)

	for i := uint64(0); i < 100; i++ {
		now := parentTime + i*params.BlockInterval/2
		for _, p := range proposers {
			ts, _, _ := sched.Timing(p, now)
			assert.True(sched.IsValid(p, ts))
		}
	}
}

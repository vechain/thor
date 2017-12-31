package schedule

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/fortest"
	"github.com/vechain/thor/params"
)

func entriesToAddrs(entries []Entry) []acc.Address {
	var addrs []acc.Address
	for _, e := range entries {
		addrs = append(addrs, e.Proposer)
	}
	return addrs
}
func TestSchedule(t *testing.T) {
	assert := assert.New(t)
	p1, p2, p3, p4, p5 :=
		acc.BytesToAddress([]byte("p1")),
		acc.BytesToAddress([]byte("p2")),
		acc.BytesToAddress([]byte("p3")),
		acc.BytesToAddress([]byte("p4")),
		acc.BytesToAddress([]byte("p5"))

	proposers := []acc.Address{p1, p2, p3, p4, p5}

	parentTime := uint64(1000)
	sched := New(
		proposers,
		nil,
		1,
		parentTime)

	entries := sched.entries

	tests := []struct {
		proposer acc.Address
		now      uint64
		ret      []interface{}
	}{
		// now time < parent block time
		{entries[1].Proposer, 0, []interface{}{
			entries[1].Time,
			entriesToAddrs(entries[:1]),
			nil,
		}},
		// now time < second entry's time
		{entries[0].Proposer, entries[1].Time - 1, []interface{}{
			entries[0].Time,
			[]acc.Address(nil),
			nil,
		}},
		// now time == second entry's time
		{entries[0].Proposer, entries[1].Time, []interface{}{
			entries[len(entries)-1].Time + params.BlockTime,
			entriesToAddrs(entries[1:]),
			nil,
		}},
		// now time == last entry's time + block time
		{entries[0].Proposer, entries[len(entries)-1].Time + params.BlockTime, []interface{}{
			entries[len(entries)-1].Time + params.BlockTime,
			entriesToAddrs(entries[1:]),
			nil,
		}},
		// now time > last entry's time + block time
		{entries[0].Proposer, entries[len(entries)-1].Time + params.BlockTime + 1, []interface{}{
			entries[len(entries)-1].Time + params.BlockTime,
			entriesToAddrs(entries[1:]),
			nil,
		}},
	}

	for _, t := range tests {
		assert.Equal(fortest.Multi(sched.Timing(t.proposer, t.now)), t.ret)

		t1, _, _ := sched.Timing(t.proposer, t.now)

		// It's guaranteed that timestamp + params.BlockTime> nowTime.
		assert.True(t1+params.BlockTime > t.now, fmt.Sprintf("ts=%d, now=%d", t1, t.now))

		// check verifiable
		t2, _, _ := sched.Timing(t.proposer, t1)
		assert.Equal(t1, t2)
	}

	{
		// not a proposer
		_, _, err := sched.Timing(acc.BytesToAddress([]byte("px")), 0)
		assert.Error(err)
	}
	{
		assert.Zero(len(New(nil, nil, 0, 0).entries))
	}
	{
		// skip absence
		sched := New(
			[]acc.Address{p1, p2, p3, p4, p5},
			[]acc.Address{p1, p2, p3, p4, p5}, 1, parentTime)

		for _, e := range sched.entries {
			assert.Equal(e.Time, sched.entries[0].Time)
		}
	}
}

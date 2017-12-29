package schedule

import (
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
		{entries[0].Proposer, 0, []interface{}{
			uint64(parentTime + params.BlockTime),
			[]acc.Address(nil),
			nil,
		}},
		{entries[0].Proposer, parentTime + params.BlockTime + params.BlockTime - 1, []interface{}{
			uint64(parentTime + params.BlockTime),
			[]acc.Address(nil),
			nil,
		}},
		{entries[0].Proposer, parentTime + params.BlockTime + params.BlockTime, []interface{}{
			uint64(parentTime + params.BlockTime*uint64(len(proposers)+1)),
			entriesToAddrs(entries[1:]),
			nil,
		}},
	}

	for _, t := range tests {
		assert.Equal(fortest.Multi(sched.Timing(t.proposer, t.now)), t.ret)
	}

}

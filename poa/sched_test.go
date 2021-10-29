// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa_test

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/thor"
)

var (
	p1 = thor.BytesToAddress([]byte("p1"))
	p2 = thor.BytesToAddress([]byte("p2"))
	p3 = thor.BytesToAddress([]byte("p3"))
	p4 = thor.BytesToAddress([]byte("p4"))
	p5 = thor.BytesToAddress([]byte("p5"))

	proposers = []poa.Proposer{
		{p1, false},
		{p2, true},
		{p3, false},
		{p4, false},
		{p5, false},
	}

	parentTime = uint64(1001)
)

func TestSchedule(t *testing.T) {

	_, err := poa.NewSchedulerV1(thor.BytesToAddress([]byte("px")), proposers, 1, parentTime)
	assert.NotNil(t, err)

	sched, _ := poa.NewSchedulerV1(p1, proposers, 1, parentTime)

	for i := uint64(0); i < 100; i++ {
		now := parentTime + i*thor.BlockInterval/2
		nbt := sched.Schedule(now)
		assert.True(t, nbt >= now)
		assert.True(t, sched.IsTheTime(nbt))
	}
}

func TestIsTheTime(t *testing.T) {
	sched, _ := poa.NewSchedulerV1(p2, proposers, 1, parentTime)

	tests := []struct {
		now  uint64
		want bool
	}{
		{parentTime - 1, false},
		{parentTime + thor.BlockInterval/2, false},
		{parentTime + thor.BlockInterval, true},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, sched.IsTheTime(tt.now))
	}
}

func TestUpdates(t *testing.T) {
	sched, _ := poa.NewSchedulerV1(p1, proposers, 1, parentTime)

	tests := []struct {
		newBlockTime uint64
		want         uint64
	}{
		{parentTime + thor.BlockInterval, 2},
		{parentTime + thor.BlockInterval*30, 1},
	}

	for _, tt := range tests {
		_, score := sched.Updates(tt.newBlockTime)
		assert.Equal(t, tt.want, score)
	}
}

func TestScheduleV2(t *testing.T) {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:], 0)
	parent := new(block.Builder).ParentID(parentID).Timestamp(parentTime).Build()

	_, err := poa.NewSchedulerV2(thor.BytesToAddress([]byte("p6")), proposers, parent.Header().Number(), parent.Header().Timestamp(), nil)
	assert.NotNil(t, err)

	sched, _ := poa.NewSchedulerV2(p2, proposers, parent.Header().Number(), parent.Header().Timestamp(), nil)

	for i := uint64(0); i < 100; i++ {
		now := parentTime + i*thor.BlockInterval/2
		nbt := sched.Schedule(now)
		assert.True(t, nbt >= now)
		assert.True(t, sched.IsTheTime(nbt))
	}
}

func TestIsTheTimeV2(t *testing.T) {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:], 0)
	parent := new(block.Builder).ParentID(parentID).Timestamp(parentTime).Build()

	sched, err := poa.NewSchedulerV2(p2, proposers, parent.Header().Number(), parent.Header().Timestamp(), nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		now  uint64
		want bool
	}{
		{parentTime - 1, false},
		{parentTime + thor.BlockInterval/2, false},
		{parentTime + thor.BlockInterval, true},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, sched.IsTheTime(tt.now))
	}
}

func TestUpdatesV2(t *testing.T) {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:], 0)
	parent := new(block.Builder).ParentID(parentID).Timestamp(parentTime).Build()

	sched, err := poa.NewSchedulerV2(p2, proposers, parent.Header().Number(), parent.Header().Timestamp(), nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		newBlockTime uint64
		want         uint64
	}{
		{parentTime + thor.BlockInterval*30, 1},
		{parentTime + thor.BlockInterval, 1},
	}

	for _, tt := range tests {
		_, score := sched.Updates(tt.newBlockTime)
		assert.Equal(t, tt.want, score)
	}
}

func TestActivateInV2(t *testing.T) {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:], 0)
	parent := new(block.Builder).ParentID(parentID).Timestamp(parentTime).Build()

	sched, err := poa.NewSchedulerV2(p1, proposers, parent.Header().Number(), parent.Header().Timestamp(), nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		newBlockTime uint64
		want         uint64
	}{
		{parentTime + thor.BlockInterval*30, 1},
		{parentTime + thor.BlockInterval, 2},
	}

	for _, tt := range tests {
		_, score := sched.Updates(tt.newBlockTime)
		assert.Equal(t, tt.want, score)
	}
}

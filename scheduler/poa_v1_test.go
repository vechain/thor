// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package scheduler

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
)

var (
	proposers = []Proposer{
		{Address: thor.BytesToAddress([]byte("p1")), Active: false, Weight: 0},
		{Address: thor.BytesToAddress([]byte("p2")), Active: true, Weight: 0},
		{Address: thor.BytesToAddress([]byte("p3")), Active: false, Weight: 0},
		{Address: thor.BytesToAddress([]byte("p4")), Active: false, Weight: 0},
		{Address: thor.BytesToAddress([]byte("p5")), Active: false, Weight: 0},
	}
	parentTimeV1 = uint64(1001)
)

func TestSchedule(t *testing.T) {
	_, err := NewPoASchedulerV1(thor.BytesToAddress([]byte("px")), proposers, 1, parentTimeV1)
	assert.NotNil(t, err)

	sched, _ := NewPoASchedulerV1(p1, proposers, 1, parentTimeV1)

	for i := range uint64(100) {
		now := parentTime + i*thor.BlockInterval()/2
		nbt := sched.Schedule(now)
		assert.True(t, nbt >= now)
		assert.True(t, sched.IsTheTime(nbt))
	}
}

func TestIsTheTime(t *testing.T) {
	sched, _ := NewPoASchedulerV1(p2, proposers, 1, parentTimeV1)

	tests := []struct {
		now  uint64
		want bool
	}{
		{parentTimeV1 - 1, false},
		{parentTimeV1 + thor.BlockInterval()/2, false},
		{parentTimeV1 + thor.BlockInterval(), true},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, sched.IsTheTime(tt.now))
	}
}

func TestUpdates(t *testing.T) {
	sched, _ := NewPoASchedulerV1(p1, proposers, 1, parentTimeV1)

	tests := []struct {
		newBlockTime uint64
		want         uint64
	}{
		{parentTimeV1 + thor.BlockInterval(), 2},
		{parentTimeV1 + thor.BlockInterval()*30, 1},
	}

	for _, tt := range tests {
		_, score := sched.Updates(tt.newBlockTime, 0)
		assert.Equal(t, tt.want, score)
	}
}

func TestScheduleV2(t *testing.T) {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:], 0)
	parent := new(block.Builder).ParentID(parentID).Timestamp(parentTimeV1).Build()

	_, err := NewPoASchedulerV2(thor.BytesToAddress([]byte("p6")), proposers, parent.Header().Number(), parent.Header().Timestamp(), nil)
	assert.NotNil(t, err)

	sched, _ := NewPoASchedulerV2(p2, proposers, parent.Header().Number(), parent.Header().Timestamp(), nil)

	for i := range uint64(100) {
		now := parentTimeV1 + i*thor.BlockInterval()/2
		nbt := sched.Schedule(now)
		assert.True(t, nbt >= now)
		assert.True(t, sched.IsTheTime(nbt))
	}
}

func TestIsTheTimeV2(t *testing.T) {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:], 0)
	parent := new(block.Builder).ParentID(parentID).Timestamp(parentTimeV1).Build()

	sched, err := NewPoASchedulerV2(p2, proposers, parent.Header().Number(), parent.Header().Timestamp(), nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		now  uint64
		want bool
	}{
		{parentTimeV1 - 1, false},
		{parentTimeV1 + thor.BlockInterval()/2, false},
		{parentTimeV1 + thor.BlockInterval(), true},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, sched.IsTheTime(tt.now))
	}
}

func TestUpdatesV2(t *testing.T) {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:], 0)
	parent := new(block.Builder).ParentID(parentID).Timestamp(parentTimeV1).Build()

	sched, err := NewPoASchedulerV2(p2, proposers, parent.Header().Number(), parent.Header().Timestamp(), nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		newBlockTime uint64
		want         uint64
	}{
		{parentTimeV1 + thor.BlockInterval()*30, 1},
		{parentTimeV1 + thor.BlockInterval(), 1},
	}

	for _, tt := range tests {
		_, score := sched.Updates(tt.newBlockTime, 0)
		assert.Equal(t, tt.want, score)
	}
}

func TestActivateInV2(t *testing.T) {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:], 0)
	parent := new(block.Builder).ParentID(parentID).Timestamp(parentTimeV1).Build()

	sched, err := NewPoASchedulerV2(p1, proposers, parent.Header().Number(), parent.Header().Timestamp(), nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		newBlockTime uint64
		want         uint64
	}{
		{parentTimeV1 + thor.BlockInterval()*30, 1},
		{parentTimeV1 + thor.BlockInterval(), 2},
	}

	for _, tt := range tests {
		_, score := sched.Updates(tt.newBlockTime, 0)
		assert.Equal(t, tt.want, score)
	}
}

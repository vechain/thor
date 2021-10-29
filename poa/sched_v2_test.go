// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"reflect"
	"testing"

	"github.com/vechain/thor/thor"
)

var (
	p1 = thor.BytesToAddress([]byte("p1"))
	p2 = thor.BytesToAddress([]byte("p2"))
	p3 = thor.BytesToAddress([]byte("p3"))
	p4 = thor.BytesToAddress([]byte("p4"))
	p5 = thor.BytesToAddress([]byte("p5"))

	parentTime = uint64(0)
)

func TestSchedulerV2_Updates(t *testing.T) {
	type fields struct {
		proposer        Proposer
		parentBlockTime uint64
		shuffled        []thor.Address
	}
	type args struct {
		newBlockTime uint64
	}
	tests := []struct {
		name        string
		fields      fields
		args        args
		wantUpdates []Proposer
		wantScore   uint64
	}{
		{"p1 should not deactivate others", fields{
			Proposer{p1, true},
			parentTime,
			[]thor.Address{p1, p2, p3, p4, p5},
		}, args{uint64(10)}, nil, 5},
		{"p1 inactive should bring p1 active", fields{
			Proposer{p1, false},
			parentTime,
			[]thor.Address{p1, p2, p3, p4, p5},
		}, args{uint64(10)}, []Proposer{{p1, true}}, 5},
		{"p2 should deactivate p1", fields{
			Proposer{p2, true},
			parentTime,
			[]thor.Address{p1, p2, p3, p4, p5},
		}, args{uint64(20)}, []Proposer{{p1, false}}, 4},
		{"p4 should deactivate nodes", fields{
			Proposer{p4, true},
			parentTime,
			[]thor.Address{p1, p2, p3, p4, p5},
		}, args{uint64(50)}, []Proposer{{p1, false}, {p2, false}, {p3, false}}, 2},
		{"long time no block should deactivate others", fields{
			Proposer{p2, true},
			parentTime,
			[]thor.Address{p1, p2, p3, p4, p5},
		}, args{uint64(200)}, []Proposer{{p1, false}, {p3, false}, {p4, false}, {p5, false}}, 1},
		{"long time no block with inactive nodes should deactivate others and bring self to active", fields{
			Proposer{p2, false},
			parentTime,
			[]thor.Address{p1, p2, p3, p4, p5},
		}, args{uint64(200)}, []Proposer{{p1, false}, {p3, false}, {p4, false}, {p5, false}, {p2, true}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SchedulerV2{
				proposer:        tt.fields.proposer,
				parentBlockTime: tt.fields.parentBlockTime,
				shuffled:        tt.fields.shuffled,
			}
			gotUpdates, gotScore := s.Updates(tt.args.newBlockTime)
			if !reflect.DeepEqual(gotUpdates, tt.wantUpdates) {
				t.Errorf("SchedulerV2.Updates() gotUpdates = %v, want %v", gotUpdates, tt.wantUpdates)
			}
			if gotScore != tt.wantScore {
				t.Errorf("SchedulerV2.Updates() gotScore = %v, want %v", gotScore, tt.wantScore)
			}
		})
	}
}

func TestNewSchedulerV2(t *testing.T) {
	seed := thor.Bytes32{}.Bytes()
	parentNumber := uint32(10)

	/*
		var ordered []thor.Address
		var list []struct {
			addr thor.Address
			hash thor.Bytes32
		}
		var addrs = []thor.Address{p1, p2, p3, p4, p5}
		var num [4]byte
		binary.BigEndian.PutUint32(num[:], parentNumber)
		for _, addr := range addrs {
			list = append(list, struct {
				addr thor.Address
				hash thor.Bytes32
			}{addr, thor.Blake2b(seed, num[:], addr.Bytes())})
		}
		sort.Slice(list, func(i, j int) bool {
			return bytes.Compare(list[i].hash.Bytes(), list[j].hash.Bytes()) < 0
		})
		for _, l := range list {
			ordered = append(ordered, l.addr)
		}
		fmt.Println(ordered)

		// get the order of shuffled list
		// p1, p4, p3, p2, p5
	*/

	type args struct {
		addr              thor.Address
		proposers         []Proposer
		parentBlockNumber uint32
		parentBlockTime   uint64
		seed              []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *SchedulerV2
		wantErr bool
	}{
		{
			"new scheduler v2",
			args{
				p1,
				[]Proposer{{p1, true}, {p2, true}, {p3, true}, {p4, true}, {p5, true}},
				parentNumber,
				parentTime,
				seed,
			},
			&SchedulerV2{Proposer{p1, true}, parentTime, []thor.Address{p1, p4, p3, p2, p5}},
			false,
		},
		{
			"self inactive should add to list",
			args{
				p1,
				[]Proposer{{p1, false}, {p2, true}, {p3, true}, {p4, true}, {p5, true}},
				parentNumber,
				parentTime,
				seed,
			},
			&SchedulerV2{Proposer{p1, false}, parentTime, []thor.Address{p1, p4, p3, p2, p5}},
			false,
		},
		{
			"not in list should throw error",
			args{
				p1,
				[]Proposer{{p2, true}, {p3, true}, {p4, true}, {p5, true}},
				parentNumber,
				parentTime,
				seed,
			},
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewSchedulerV2(tt.args.addr, tt.args.proposers, tt.args.parentBlockNumber, tt.args.parentBlockTime, tt.args.seed)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSchedulerV2() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewSchedulerV2() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSchedulerV2_Schedule(t *testing.T) {
	type fields struct {
		proposer        Proposer
		parentBlockTime uint64
		shuffled        []thor.Address
	}
	type args struct {
		nowTime uint64
	}
	tests := []struct {
		name             string
		fields           fields
		args             args
		wantNewBlockTime uint64
	}{
		{"p1 should schedule 10", fields{
			Proposer{p1, true},
			parentTime,
			[]thor.Address{p1, p2, p3, p4, p5},
		}, args{uint64(5)}, 10},
		{"p2 should schedule 20", fields{
			Proposer{p2, true},
			parentTime,
			[]thor.Address{p1, p2, p3, p4, p5},
		}, args{uint64(5)}, 20},
		{"p5 should schedule 50", fields{
			Proposer{p5, true},
			parentTime,
			[]thor.Address{p1, p2, p3, p4, p5},
		}, args{uint64(5)}, 50},
		{"p1 at 15 should schedule 60", fields{
			Proposer{p1, true},
			parentTime,
			[]thor.Address{p1, p2, p3, p4, p5},
		}, args{uint64(15)}, 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SchedulerV2{
				proposer:        tt.fields.proposer,
				parentBlockTime: tt.fields.parentBlockTime,
				shuffled:        tt.fields.shuffled,
			}
			if gotNewBlockTime := s.Schedule(tt.args.nowTime); gotNewBlockTime != tt.wantNewBlockTime {
				t.Errorf("SchedulerV2.Schedule() = %v, want %v", gotNewBlockTime, tt.wantNewBlockTime)
			}
		})
	}
}

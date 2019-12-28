package logdb

import "testing"
import "math"

func TestSequence(t *testing.T) {
	type args struct {
		blockNum uint32
		index    uint32
	}
	tests := []struct {
		name string
		args args
		want args
	}{
		{"regular", args{1, 2}, args{1, 2}},
		{"max bn", args{math.MaxUint32, 1}, args{math.MaxUint32, 1}},
		{"max index", args{5, math.MaxInt32}, args{5, math.MaxInt32}},
		{"both max", args{math.MaxUint32, math.MaxInt32}, args{math.MaxUint32, math.MaxInt32}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newSequence(tt.args.blockNum, tt.args.index)
			if bn := got.BlockNumber(); bn != tt.want.blockNum {
				t.Errorf("seq.blockNum() = %v, want %v", bn, tt.want.blockNum)
			}
			if i := got.Index(); i != tt.want.index {
				t.Errorf("seq.index() = %v, want %v", i, tt.want.index)
			}
		})
	}

	defer func() {
		if e := recover(); e == nil {
			t.Errorf("newSequence should panic on 2nd arg > math.MaxInt32")
		}
	}()
	newSequence(1, math.MaxInt32+1)
}

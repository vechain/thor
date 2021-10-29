// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"sync/atomic"
	"testing"

	"github.com/vechain/thor/thor"
)

func TestHeader_BetterThan(t *testing.T) {
	type fields struct {
		body  headerBody
		cache struct {
			signingHash atomic.Value
			id          atomic.Value
			pubkey      atomic.Value
			beta        atomic.Value
		}
	}
	type args struct {
		other *Header
	}

	var (
		largerID  fields
		smallerID fields
	)
	largerID.cache.id.Store(thor.Bytes32{1})
	smallerID.cache.id.Store(thor.Bytes32{0})

	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{"higher score", fields{body: headerBody{TotalScore: 10}}, args{other: &Header{body: headerBody{TotalScore: 9}}}, true},
		{"lower score", fields{body: headerBody{TotalScore: 9}}, args{other: &Header{body: headerBody{TotalScore: 10}}}, false},
		{"equal score, larger id", largerID, args{&Header{smallerID.body, smallerID.cache}}, false},
		{"equal score, smaller id", smallerID, args{&Header{largerID.body, largerID.cache}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{
				body:  tt.fields.body,
				cache: tt.fields.cache,
			}
			if got := h.BetterThan(tt.args.other); got != tt.want {
				t.Errorf("Header.BetterThan() = %v, want %v", got, tt.want)
			}
		})
	}
}

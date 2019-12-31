// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package chain

import (
	"testing"

	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
)

func TestChain_HasBlock(t *testing.T) {
	type fields struct {
		repo   *Repository
		headID thor.Bytes32
		init   func() (*muxdb.Trie, error)
	}
	type args struct {
		id thor.Bytes32
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Chain{
				repo:     tt.fields.repo,
				headID:   tt.fields.headID,
				lazyInit: tt.fields.init,
			}
			got, err := c.HasBlock(tt.args.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("Chain.HasBlock() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Chain.HasBlock() = %v, want %v", got, tt.want)
			}
		})
	}
}

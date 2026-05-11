// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
)

func TestParseBytes32Compact(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    thor.Bytes32
		wantErr bool
	}{
		{
			name:  "compact zero",
			input: "0x0",
			want:  thor.Bytes32{},
		},
		{
			name:  "compact odd hex",
			input: "0xa",
			want:  thor.Bytes32{31: 0x0a},
		},
		{
			name:  "compact two bytes",
			input: "0x1234",
			want:  thor.Bytes32{30: 0x12, 31: 0x34},
		},
		{
			name:  "full 64 hex chars with prefix",
			input: "0x" + "11223344556677889900aabbccddeeff" + "11223344556677889900aabbccddeeff",
			want: thor.Bytes32{
				0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
				0x99, 0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
				0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
				0x99, 0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
			},
		},
		{
			name:    "missing 0x prefix",
			input:   "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBytes32Compact(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

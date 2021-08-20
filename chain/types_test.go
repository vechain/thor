// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package chain

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
)

func Test_extension_EncodeRLP(t *testing.T) {
	type fields struct {
		Beta []byte
	}

	tests := []struct {
		name    string
		fields  fields
		wantW   string
		wantErr bool
	}{
		{"emptyBeta", fields{[]byte{}}, "0x", false},
		{"notEmptyBeta", fields{[]byte{0x1}}, "0xc101", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ex := &extension{
				Beta: tt.fields.Beta,
			}
			w := &bytes.Buffer{}
			if err := ex.EncodeRLP(w); (err != nil) != tt.wantErr {
				t.Errorf("extension.EncodeRLP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotW := hexutil.Encode(w.Bytes()); gotW != tt.wantW {
				t.Errorf("extension.EncodeRLP() = %v, want %v", gotW, tt.wantW)
			}
		})
	}
}

func Test_extension_DecodeRLP(t *testing.T) {
	type container struct {
		Extension extension
	}

	tests := []struct {
		name    string
		encoded string
		wantW   container
		wantErr bool
	}{
		{"emptyBeta", "0xc0", container{extension{[]byte{}}}, false},
		{"notEmptyBeta", "0xc2c101", container{extension{[]byte{0x1}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ex container

			if err := rlp.DecodeBytes(hexutil.MustDecode(tt.encoded), &ex); (err != nil) != tt.wantErr {
				t.Errorf("extension.DecodeRLP() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !bytes.Equal(tt.wantW.Extension.Beta, ex.Extension.Beta) {
				t.Errorf("extension.DecodeRLP() = %v, want %v", hexutil.Encode(ex.Extension.Beta), hexutil.Encode(tt.wantW.Extension.Beta))
			}

		})
	}
}

func Test_extension_Decode_not_trimmed(t *testing.T) {
	type container struct {
		Extention struct {
			Beta []byte
		}
	}

	var c container
	data, err := rlp.EncodeToBytes(&c)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(hexutil.Encode(data))
	var ex struct {
		Extension extension
	}
	err = rlp.DecodeBytes(data, &ex)

	assert.EqualError(t, err, "rlp(BlockSummary): extension must be trimmed")
}

// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
)

func TestReservedEncoding(t *testing.T) {
	cases := []struct {
		input    reserved
		expected []byte
	}{
		{reserved{0, nil}, []byte{0xc0}},
		{reserved{8, nil}, []byte{0xc1, 0x08}},
		{reserved{8, []rlp.RawValue{[]byte{0x81}}}, []byte{0xc2, 0x08, 0x81}},
		{reserved{8, []rlp.RawValue{[]byte{0x80}}}, []byte{0xc1, 0x08}}, // trimmed
		{reserved{8, []rlp.RawValue{[]byte{0xc0}}}, []byte{0xc1, 0x08}}, // trimmed
	}

	for i, c := range cases {
		data, err := rlp.EncodeToBytes(&c.input)
		assert.Nil(t, err, "case #%v", i)
		assert.Equal(t, c.expected, data, "case #%v", i)
	}
}

func TestReservedCountLimit(t *testing.T) {
	// MaxUnusedReservedFields+1 unused fields (MaxUnusedReservedFields+2 raws including Features) must be rejected.
	n := MaxUnusedReservedFields + 2
	raws := make([]rlp.RawValue, n)
	for i := range raws {
		raws[i] = rlp.RawValue{0x01}
	}
	data, err := rlp.EncodeToBytes(raws)
	assert.NoError(t, err)

	var r reserved
	err = rlp.DecodeBytes(data, &r)
	assert.ErrorContains(t, err, "reserved field count exceeds limit")

	// Exactly at limit (MaxUnusedReservedFields unused + 1 Features) must pass.
	raws = make([]rlp.RawValue, MaxUnusedReservedFields+1)
	for i := range raws {
		raws[i] = rlp.RawValue{0x01}
	}
	data, err = rlp.EncodeToBytes(raws)
	assert.NoError(t, err)
	err = rlp.DecodeBytes(data, &r)
	assert.NoError(t, err)
}

func TestReservedDecoding(t *testing.T) {
	cases := []struct {
		input    []byte
		expected reserved
	}{
		{[]byte{0xc0}, reserved{0, nil}},
		{[]byte{0xc1, 0x08}, reserved{8, nil}},
		{[]byte{0xc2, 0x08, 0x07}, reserved{8, []rlp.RawValue{[]byte{0x07}}}},
	}

	for i, c := range cases {
		var r reserved
		err := rlp.DecodeBytes(c.input, &r)
		assert.Nil(t, err, "case #%v", i)
		assert.Equal(t, c.expected, r, "case #%v", i)
	}

	var r reserved
	err := rlp.DecodeBytes([]byte{0xc1, 0x80}, &r)
	assert.EqualError(t, err, "invalid reserved fields: not trimmed")

	err = rlp.DecodeBytes([]byte{0xc2, 0x1, 0x80}, &r)
	assert.EqualError(t, err, "invalid reserved fields: not trimmed")
}


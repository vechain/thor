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

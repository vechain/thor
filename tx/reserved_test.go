package tx

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
)

func TestReserved(t *testing.T) {
	r := reserved{
		Features: 1,
		Unused:   nil,
	}

	data, err := rlp.EncodeToBytes(&r)
	assert.Nil(t, err)

	expected, _ := rlp.EncodeToBytes([]interface{}{Features(1)})
	assert.EqualValues(t, expected, data)

	// trimming
	r.Unused = []rlp.RawValue{rlp.EmptyList}
	data, err = rlp.EncodeToBytes(&r)
	assert.Nil(t, err)
	assert.EqualValues(t, expected, data)

	// trimming
	r.Unused = []rlp.RawValue{rlp.EmptyString}
	data, err = rlp.EncodeToBytes(&r)
	assert.Nil(t, err)
	assert.EqualValues(t, expected, data)

	// trimming
	r.Features = 0
	data, err = rlp.EncodeToBytes(&r)
	assert.Nil(t, err)
	assert.EqualValues(t, rlp.EmptyList, data)
}

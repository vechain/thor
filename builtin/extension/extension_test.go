package extension

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func TestExtension(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Bytes32{}, kv)

	acc := thor.BytesToAddress([]byte("a1"))

	ext := New(acc, st)

	expected1, _ := hex.DecodeString(`f5d67bae73b0e10d0dfd3043b3f4f100ada014c5c37bd5ce97813b13f5ab2bcf`)
	expected2, _ := hex.DecodeString(`bddd813c634239723171ef3fee98579b94964e3bb1cb3e427262c8c068d52319`)
	expected3, _ := hex.DecodeString(`9af4ece202568e3585b4dc02f3c1751149eae7cff55d087076e2769483b51b83`)

	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{ext.Blake2b256([]byte("123")), thor.BytesToBytes32(expected1)},
		{ext.Blake2b256([]byte("abc")), thor.BytesToBytes32(expected2)},
		{ext.Blake2b256([]byte("123abc")), thor.BytesToBytes32(expected3)},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}

}

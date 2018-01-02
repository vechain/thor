package bn_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/bn"
	"github.com/vechain/thor/fortest"
)

func TestInt(t *testing.T) {

	assert := assert.New(t)

	assert.Equal(bn.FromBig(big.NewInt(1)).ToBig(), big.NewInt(1))

	i := bn.Int{}
	i.SetBig(big.NewInt(1))
	assert.Equal(i, bn.FromBig(big.NewInt(1)))

	assert.True(bn.Int{}.IsZero())

	{
		tests := []struct {
			v1     bn.Int
			v2     bn.Int
			expect int
		}{
			{bn.Int{}, bn.Int{}, 0},
			{bn.Int{}, bn.FromBig(big.NewInt(1)), -1},
			{bn.Int{}, bn.FromBig(big.NewInt(-1)), 1},
			{bn.FromBig(big.NewInt(1)), bn.Int{}, 1},
			{bn.FromBig(big.NewInt(-1)), bn.Int{}, -1},
			{bn.FromBig(big.NewInt(-1)), bn.FromBig(big.NewInt(1)), -1},
		}
		for _, test := range tests {
			assert.Equal(test.expect, test.v1.Cmp(test.v2))
		}
	}
	{
		tests := []struct {
			v1     bn.Int
			v2     *big.Int
			expect int
		}{
			{bn.Int{}, big.NewInt(0), 0},
			{bn.Int{}, big.NewInt(1), -1},
			{bn.Int{}, big.NewInt(-1), 1},
			{bn.FromBig(big.NewInt(1)), big.NewInt(0), 1},
			{bn.FromBig(big.NewInt(-1)), big.NewInt(0), -1},
		}
		for _, test := range tests {
			assert.Equal(test.expect, test.v1.CmpBig(test.v2))
		}
	}

}

func TestIntEncoding(t *testing.T) {

	tests := []struct {
		v1 bn.Int
		v2 *big.Int
	}{
		{bn.Int{}, new(big.Int)},
		{bn.FromBig(big.NewInt(12345)), big.NewInt(12345)},
		{bn.FromBig(big.NewInt(-12345)), big.NewInt(-12345)},
	}

	for _, test := range tests {
		assert.Equal(t, test.v1.String(), test.v2.String(), "String() should be equal")

		assert.Equal(t,
			fortest.Multi(rlp.EncodeToBytes(&test.v1)),
			fortest.Multi(rlp.EncodeToBytes(test.v2)),
			"rlp encoded should be equal")
		assert.Equal(t,
			fortest.Multi(test.v1.MarshalText()),
			fortest.Multi(test.v2.MarshalText()),
			"json marshaled should be equal")
		assert.Equal(t,
			fortest.Multi(json.Marshal(&test.v1)),
			fortest.Multi(json.Marshal(test.v2)),
			"json marshaled should be equal")
	}
}

func TestDecoding(t *testing.T) {

	v := bn.Int{}
	assert.NoError(t, rlp.DecodeBytes([]byte{128}, &v))
	assert.Equal(t, v.ToBig(), big.NewInt(0))
	assert.NoError(t, rlp.DecodeBytes([]byte{0x10}, &v))
	assert.Equal(t, v.ToBig(), big.NewInt(0x10))

	assert.NoError(t, v.UnmarshalText([]byte("123")))
	assert.Equal(t, v.ToBig(), big.NewInt(123))

	assert.NoError(t, json.Unmarshal([]byte("1234"), &v))
	assert.Equal(t, v.ToBig(), big.NewInt(1234))

	assert.NoError(t, json.Unmarshal([]byte("null"), &v))
	assert.Equal(t, v.ToBig(), new(big.Int))

}

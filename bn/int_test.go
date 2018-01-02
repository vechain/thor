package bn_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/bn"
)

func TestInt(t *testing.T) {
	assert := assert.New(t)
	i1 := bn.Int{}
	i2 := i1

	i1.SetBig(big.NewInt(1))
	assert.True(i1.Cmp(i2) > 0)
}

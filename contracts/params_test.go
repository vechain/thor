package contracts_test

import (
	"math/big"
	"testing"

	"github.com/vechain/thor/thor"

	. "github.com/vechain/thor/contracts"
)

func TestParams(t *testing.T) {
	Params.RuntimeBytecodes()
	Params.PackGet("k")
	Params.PackSet("k", &big.Int{})
	Params.PackInitialize(thor.Address{})
	Params.UnpackGet(make([]byte, 32))
}

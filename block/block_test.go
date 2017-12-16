package block_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	. "github.com/vechain/thor/block"
)

func TestBlock(t *testing.T) {

	block := new(Builder).
		GasUsed(big.NewInt(1000)).
		Build()
	h := block.Header()
	fmt.Println(h.Hash())

	data, _ := rlp.EncodeToBytes(block)
	fmt.Println(data)

	b := Block{}
	rlp.DecodeBytes(data, &b)
	fmt.Println(b.Header().Hash())

}

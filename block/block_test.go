package block_test

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	. "github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestBlock(t *testing.T) {

	tx1 := new(tx.Builder).Clause(tx.NewClause(&thor.Address{})).Clause(tx.NewClause(&thor.Address{})).Build()
	tx2 := new(tx.Builder).Clause(tx.NewClause(nil)).Build()
	block := new(Builder).
		GasUsed(1000).
		Transaction(tx1).
		Transaction(tx2).
		Build()
	h := block.Header()
	fmt.Println(h.ID())

	data, _ := rlp.EncodeToBytes(block)
	fmt.Println(Raw(data).DecodeHeader())
	fmt.Println(Raw(data).DecodeBody())

	b := Block{}
	rlp.DecodeBytes(data, &b)
	fmt.Println(b.Header().ID())
	fmt.Println(&b)
	fmt.Println(&b)
}

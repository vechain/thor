package tx_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	. "github.com/vechain/thor/tx"
)

func TestTx(t *testing.T) {
	assert := assert.New(t)

	tx1 := new(Builder).
		Nonce(1).
		Gas(100).
		Clause(&Clause{}).
		Build()

	data1, _ := rlp.EncodeToBytes(tx1)

	tx2 := &Transaction{}

	rlp.DecodeBytes(data1, tx2)
	data2, _ := rlp.EncodeToBytes(&tx2)
	assert.Equal(data1, data2)
}

func BenchmarkTxMining(b *testing.B) {
	builder := new(tx.Builder)
	signer := thor.BytesToAddress([]byte("acc1"))
	maxWork := &big.Int{}
	for i := 0; i < b.N; i++ {
		tx := builder.Nonce((uint64(i))).Build()
		work := tx.EvaluateWork(signer)
		if work.Cmp(maxWork) > 0 {
			maxWork = work
		}
	}

	b.Log(maxWork)
}

func TestClause(t *testing.T) {
	fmt.Println(tx.NewClause(nil))
	c1 := tx.NewClause(nil)
	tx := new(tx.Builder).Clause(c1).Clause(c1).Build()
	fmt.Println(tx)
}

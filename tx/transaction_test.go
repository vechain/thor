package tx_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
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

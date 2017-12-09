package tx_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	. "github.com/vechain/thor/tx"
)

func TestTx(t *testing.T) {
	assert := assert.New(t)

	b := Builder{}
	tx := b.Nonce(1).GasLimit(big.NewInt(100)).Clause(&Clause{}).Build()
	data, _ := rlp.EncodeToBytes(tx)

	tx2 := Transaction{}

	rlp.DecodeBytes(data, &tx2)
	data2, _ := rlp.EncodeToBytes(&tx2)
	assert.Equal(data, data2)

}

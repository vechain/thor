package tx_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	. "github.com/vechain/vecore/tx"
)

func TestTx(t *testing.T) {
	assert := assert.New(t)

	b := Builder{}
	tx := b.Nonce(1).GasLimit(big.NewInt(100)).Clause(&Clause{}).Build()
	data, _ := rlp.EncodeToBytes(tx)

	var dec Decoder
	rlp.DecodeBytes(data, &dec)
	data2, _ := rlp.EncodeToBytes(dec.Result)
	assert.Equal(data, data2)

}

package processor_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	. "github.com/vechain/thor/processor"
	"github.com/vechain/thor/vm"
)

func TestExecuteMsg(t *testing.T) {
	assert := assert.New(t)
	sender := acc.Address(crypto.PubkeyToAddress(key.PublicKey))
	state := NewState()
	state.SetOwner(sender)
	block := new(block.Builder).Beneficiary(sender).Timestamp(uint64(time.Now().Unix())).Transaction(buildTransaction()).Build()
	header := block.Header()
	transaction := block.Transactions()[0]
	context := NewContext(big.NewInt(1), sender, header, 5000000000000000000, transaction.Hash(), state, nil)

	messages, _ := transaction.AsMessages()
	msg := messages[0]

	_, gasUsed := ExecuteMsg(msg, vm.Config{}, context)
	//t.Log(output)
	assert.Equal(gasUsed.Int64(), int64(79524))
}

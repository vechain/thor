package processor

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/vm"
)

func TestExecuteMsg(t *testing.T) {
	sender := acc.Address(crypto.PubkeyToAddress(key.PublicKey))
	state := NewState()
	state.SetOwner(sender)
	block := new(block.Builder).Beneficiary(sender).Timestamp(uint64(time.Now().Unix())).Transaction(buildTransaction()).Build()
	header := block.Header()

	context := Context{
		price:    big.NewInt(1),
		sender:   sender,
		header:   header,
		gasLimit: 4999999999999921932,
		txHash:   cry.Hash{1},
		state:    state,
		kv:       nil,
		getHash:  nil,
	}

	transaction := block.Transactions()[0]
	messages, _ := transaction.AsMessages()
	msg := messages[0]

	output := ExecuteMsg(msg, vm.Config{}, context)
	t.Log(output)
}

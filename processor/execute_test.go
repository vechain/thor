package processor_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	. "github.com/vechain/thor/processor"
	"github.com/vechain/thor/vm"
)

func TestExecuteMsg(t *testing.T) {
	sender := acc.Address(crypto.PubkeyToAddress(key.PublicKey))
	state := NewState()
	state.SetOwner(sender)
	block := new(block.Builder).Beneficiary(sender).Timestamp(uint64(time.Now().Unix())).Transaction(buildTransaction()).Build()
	header := block.Header()
	context := NewContext(big.NewInt(1), sender, header, 4999999999999921932, cry.Hash{1}, state, nil, nil)

	transaction := block.Transactions()[0]
	messages, _ := transaction.AsMessages()
	msg := messages[0]

	output := ExecuteMsg(msg, vm.Config{}, context)
	t.Log(output)
}

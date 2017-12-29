package processor_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/lvldb"
	. "github.com/vechain/thor/processor"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/vm"
)

func TestExecuteMsg(t *testing.T) {
	assert := assert.New(t)

	db, _ := lvldb.NewMem()
	state, _ := state.New(cry.Hash{}, db)
	defer db.Close()

	sender := acc.Address(crypto.PubkeyToAddress(key.PublicKey))
	state.SetBalance(sender, big.NewInt(5000000000000000000))

	block := new(block.Builder).Beneficiary(sender).Timestamp(uint64(time.Now().Unix())).Transaction(buildTransaction()).Build()
	header := block.Header()
	transaction := block.Transactions()[0]
	context := NewContext(big.NewInt(1), sender, header, 5000000000000000000, transaction.Hash(), state, nil)

	messages, _ := transaction.AsMessages()
	msg := messages[0]

	_, gasUsed := ExecuteMsg(msg, vm.Config{}, context)
	assert.Equal(gasUsed.Int64(), int64(79524))
}

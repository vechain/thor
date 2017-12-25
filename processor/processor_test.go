package processor_test

import (
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/dsa"
	. "github.com/vechain/thor/processor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

type accountFake struct {
	balance *big.Int
	code    []byte
}

// State implement Stater.
type State struct {
	accounts map[acc.Address]*accountFake
	storages map[cry.Hash]cry.Hash
}

// NewState mock Stater interface.
func NewState() *State {
	state := &State{
		make(map[acc.Address]*accountFake),
		make(map[cry.Hash]cry.Hash),
	}
	return state
}

// SetOwner mock a rich account.
func (st *State) SetOwner(addr acc.Address) {
	st.accounts[addr] = &accountFake{
		balance: big.NewInt(5000000000000000000),
	}
}

func (st *State) Error() error {
	return nil
}

func (st *State) Exist(addr acc.Address) bool {
	if acc := st.accounts[addr]; acc == nil {
		st.accounts[addr] = &accountFake{
			new(big.Int),
			[]byte{},
		}
	}
	return true
}

func (st *State) GetStorage(addr acc.Address, key cry.Hash) cry.Hash {
	if storage := st.storages[key]; storage != (cry.Hash{}) {
		return storage
	}
	newST := cry.Hash{}
	st.storages[key] = newST
	return newST
}

func (st *State) GetBalance(addr acc.Address) *big.Int {
	return st.accounts[addr].balance
}

func (st *State) GetCode(addr acc.Address) []byte {
	return st.accounts[addr].code
}

func (st *State) SetBalance(addr acc.Address, balance *big.Int) {
	st.accounts[addr].balance = balance
}

func (st *State) SetCode(addr acc.Address, code []byte) {
	st.accounts[addr].code = code
}

func (st *State) SetStorage(addr acc.Address, key cry.Hash, value cry.Hash) {
	st.storages[key] = value
}

func (st *State) DeleteAccount(addr acc.Address) {
	st.accounts[addr] = nil
}

var key = func() *ecdsa.PrivateKey {
	key, _ := crypto.GenerateKey()
	return key
}()

func TestHandleTransaction(t *testing.T) {
	assert := assert.New(t)
	sender := acc.Address(crypto.PubkeyToAddress(key.PublicKey))
	state := NewState()
	state.SetOwner(sender)
	processor := New(state, nil)
	block := new(block.Builder).Beneficiary(sender).Timestamp(uint64(time.Now().Unix())).Transaction(buildTransaction()).Build()
	header := block.Header()
	transaction := block.Transactions()[0]
	_, gasUsed, _ := processor.Process(header, transaction, vm.Config{})
	//t.Log(outputs[0])
	assert.Equal(gasUsed.Int64(), int64(157592))
}

func buildTransaction() *tx.Transaction {
	createClause := &tx.Clause{
		To:   nil,
		Data: common.Hex2Bytes("6060604052341561000f57600080fd5b61018d8061001e6000396000f30060606040526004361061006d576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806316e64048146100725780631f2a63c01461009b57806344650b74146100c457806367e06858146100e75780639650470c14610110575b600080fd5b341561007d57600080fd5b610085610133565b6040518082815260200191505060405180910390f35b34156100a657600080fd5b6100ae610139565b6040518082815260200191505060405180910390f35b34156100cf57600080fd5b6100e5600480803590602001909190505061013f565b005b34156100f257600080fd5b6100fa610149565b6040518082815260200191505060405180910390f35b341561011b57600080fd5b6101316004808035906020019091905050610157565b005b60005481565b60015481565b8060008190555050565b600060015460005401905090565b80600181905550505600a165627a7a723058201fb67fe068c521eaa014e518e0916c231e5e376eeabcad865a6c8a8619c34fca0029"),
	}

	tx := new(tx.Builder).GasPrice(big.NewInt(1)).GasLimit(big.NewInt(5000000000000000000)).Clause(createClause).Build()
	sig, _ := dsa.Sign(tx.HashForSigning(), crypto.FromECDSA(key))
	return tx.WithSignature(sig)
}

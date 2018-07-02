// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime_test

import (
	"encoding/hex"
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/xenv"
)

func TestContractSuicide(t *testing.T) {
	assert := assert.New(t)
	kv, _ := lvldb.NewMem()

	g := genesis.NewDevnet()
	stateCreator := state.NewCreator(kv)
	b0, _, err := g.Build(stateCreator)
	if err != nil {
		t.Fatal(err)
	}
	ch, _ := chain.New(kv, b0)

	// contract:
	//
	// pragma solidity ^0.4.18;

	// contract TestSuicide {
	// 	function testSuicide() public {
	// 		selfdestruct(msg.sender);
	// 	}
	// }
	data, _ := hex.DecodeString("608060405260043610603f576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff168063085da1b3146044575b600080fd5b348015604f57600080fd5b5060566058565b005b3373ffffffffffffffffffffffffffffffffffffffff16ff00a165627a7a723058204cb70b653a3d1821e00e6ade869638e80fa99719931c9fa045cec2189d94086f0029")
	time := b0.Header().Timestamp()
	addr := thor.BytesToAddress([]byte("acc01"))
	state, _ := stateCreator.NewState(b0.Header().StateRoot())
	state.SetCode(addr, data)
	state.SetEnergy(addr, big.NewInt(100), time)
	state.SetBalance(addr, big.NewInt(200))

	abi, _ := abi.New([]byte(`[{
			"constant": false,
			"inputs": [],
			"name": "testSuicide",
			"outputs": [],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`))
	suicide, _ := abi.MethodByName("testSuicide")
	methodData, err := suicide.EncodeInput()
	if err != nil {
		t.Fatal(err)
	}

	origin := genesis.DevAccounts()[0].Address
	out := runtime.New(ch.NewSeeker(b0.Header().ID()), state, &xenv.BlockContext{Time: time}).
		ExecuteClause(tx.NewClause(&addr).WithData(methodData), 0, math.MaxUint64, &xenv.TransactionContext{Origin: origin})
	if out.VMErr != nil {
		t.Fatal(out.VMErr)
	}

	expectedTransfer := &tx.Transfer{
		Sender:    addr,
		Recipient: origin,
		Amount:    big.NewInt(200),
	}
	assert.Equal(1, len(out.Transfers))
	assert.Equal(expectedTransfer, out.Transfers[0])

	event, _ := builtin.Energy.ABI.EventByName("Transfer")
	expectedEvent := &tx.Event{
		Address: builtin.Energy.Address,
		Topics:  []thor.Bytes32{event.ID(), thor.BytesToBytes32(addr.Bytes()), thor.BytesToBytes32(origin.Bytes())},
		Data:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 100},
	}
	assert.Equal(1, len(out.Events))
	assert.Equal(expectedEvent, out.Events[0])

	assert.Equal(big.NewInt(0), state.GetBalance(addr))
	assert.Equal(big.NewInt(0), state.GetEnergy(addr, time))

	bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
	assert.Equal(new(big.Int).Add(bal, big.NewInt(200)), state.GetBalance(origin))
	assert.Equal(new(big.Int).Add(bal, big.NewInt(100)), state.GetEnergy(origin, time))
}

func TestCall(t *testing.T) {
	kv, _ := lvldb.NewMem()

	g := genesis.NewDevnet()
	b0, _, err := g.Build(state.NewCreator(kv))
	if err != nil {
		t.Fatal(err)
	}

	ch, _ := chain.New(kv, b0)

	state, _ := state.New(b0.Header().StateRoot(), kv)

	rt := runtime.New(ch.NewSeeker(b0.Header().ID()), state, &xenv.BlockContext{})

	method, _ := builtin.Params.ABI.MethodByName("executor")
	data, err := method.EncodeInput()
	if err != nil {
		t.Fatal(err)
	}

	out := rt.ExecuteClause(
		tx.NewClause(&builtin.Params.Address).WithData(data),
		0, math.MaxUint64, &xenv.TransactionContext{})

	if out.VMErr != nil {
		t.Fatal(out.VMErr)
	}

	var addr common.Address
	if err := method.DecodeOutput(out.Data, &addr); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, thor.Address(addr), genesis.DevAccounts()[0].Address)

	// contract NeverStop {
	// 	constructor() public {
	// 		while(true) {
	// 		}
	// 	}
	// }
	data, _ = hex.DecodeString("6080604052348015600f57600080fd5b505b600115601b576011565b60358060286000396000f3006080604052600080fd00a165627a7a7230582026c386600e61384b3a93bf45760f3207b5cac072cec31c9cea1bc7099bda49b00029")
	exec, interrupt := rt.PrepareClause(tx.NewClause(nil).WithData(data), 0, math.MaxUint64, &xenv.TransactionContext{})

	go func() {
		interrupt()
	}()

	out, interrupted := exec()
	assert.NotNil(t, out)
	assert.True(t, interrupted)
}

func TestExecuteTransaction(t *testing.T) {

	// kv, _ := lvldb.NewMem()

	// key, _ := crypto.GenerateKey()
	// addr1 := thor.Address(crypto.PubkeyToAddress(key.PublicKey))
	// addr2 := thor.BytesToAddress([]byte("acc2"))
	// balance1 := big.NewInt(1000 * 1000 * 1000)

	// b0, err := new(genesis.Builder).
	// 	Alloc(contracts.Energy.Address, &big.Int{}, contracts.Energy.RuntimeBytecodes()).
	// 	Alloc(addr1, balance1, nil).
	// 	Call(contracts.Energy.PackCharge(addr1, big.NewInt(1000000))).
	// 	Build(state.NewCreator(kv))

	// if err != nil {
	// 	t.Fatal(err)
	// }

	// tx := new(tx.Builder).
	// 	GasPrice(big.NewInt(1)).
	// 	Gas(1000000).
	// 	Clause(tx.NewClause(&addr2).WithValue(big.NewInt(10))).
	// 	Build()

	// sig, _ := crypto.Sign(tx.SigningHash().Bytes(), key)
	// tx = tx.WithSignature(sig)

	// state, _ := state.New(b0.Header().StateRoot(), kv)
	// rt := runtime.New(state,
	// 	thor.Address{}, 0, 0, 0, func(uint32) thor.Bytes32 { return thor.Bytes32{} })
	// receipt, _, err := rt.ExecuteTransaction(tx)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// _ = receipt
	// assert.Equal(t, state.GetBalance(addr1), new(big.Int).Sub(balance1, big.NewInt(10)))
}

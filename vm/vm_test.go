// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package vm_test

import (
	"math"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	dbstate "github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	. "github.com/vechain/thor/vm"
)

func newEnv() *VM {
	db, _ := lvldb.NewMem()
	statr, _ := dbstate.New(thor.Hash{}, db)
	ctx := Context{
		TxID:        thor.Hash{1},
		ClauseIndex: 1,
		GetHash: func(n uint32) thor.Hash {
			return thor.BytesToHash(crypto.Keccak256([]byte(new(big.Int).SetUint64(uint64(n)).String())))
		},
		BlockNumber: 0,
		GasPrice:    nil,
		Time:        uint64(time.Now().Unix()),
		GasLimit:    math.MaxUint64,
	}
	return New(ctx, statr, Config{})
}

func TestContract(t *testing.T) {
	assert := assert.New(t)

	origin := thor.BytesToAddress([]byte("0x0a"))
	env := newEnv()

	// 创建合约
	var input = common.Hex2Bytes("6060604052341561000f57600080fd5b61018d8061001e6000396000f30060606040526004361061006d576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806316e64048146100725780631f2a63c01461009b57806344650b74146100c457806367e06858146100e75780639650470c14610110575b600080fd5b341561007d57600080fd5b610085610133565b6040518082815260200191505060405180910390f35b34156100a657600080fd5b6100ae610139565b6040518082815260200191505060405180910390f35b34156100cf57600080fd5b6100e5600480803590602001909190505061013f565b005b34156100f257600080fd5b6100fa610149565b6040518082815260200191505060405180910390f35b341561011b57600080fd5b6101316004808035906020019091905050610157565b005b60005481565b60015481565b8060008190555050565b600060015460005401905090565b80600181905550505600a165627a7a723058201fb67fe068c521eaa014e518e0916c231e5e376eeabcad865a6c8a8619c34fca0029")
	output := env.Create(origin, input, math.MaxUint64, new(big.Int))
	contractAddr := *output.ContractAddress
	if output.VMErr != nil {
		t.Fatal("didn't expect error:", output.VMErr)
	}

	// 创建 abi
	var definition = `[{"constant":true,"inputs":[],"name":"left","outputs":[{"name":"","type":"int256"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[],"name":"right","outputs":[{"name":"","type":"int256"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":false,"inputs":[{"name":"num","type":"int256"}],"name":"Left","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":false,"inputs":[],"name":"Add","outputs":[{"name":"","type":"int256"}],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":false,"inputs":[{"name":"num","type":"int256"}],"name":"Right","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"}]`
	abi, err := abi.JSON(strings.NewReader(definition))
	if err != nil {
		t.Fatal(err)
	}

	// 设置 left
	left, err := abi.Pack("Left", big.NewInt(5))
	if err != nil {
		t.Fatal(err)
	}
	output = env.Call(origin, contractAddr, left, math.MaxUint64, new(big.Int))
	if output.VMErr != nil {
		t.Fatal("didn't expect error:", output.VMErr)
	}

	// 设置 right
	right, err := abi.Pack("Right", big.NewInt(6))
	if err != nil {
		t.Fatal(err)
	}
	output = env.Call(origin, contractAddr, right, math.MaxUint64, new(big.Int))
	if output.VMErr != nil {
		t.Fatal("didn't expect error:", output.VMErr)
	}

	// ADD
	add, err := abi.Pack("Add")
	if err != nil {
		t.Fatal(err)
	}
	output = env.Call(origin, contractAddr, add, math.MaxUint64, new(big.Int))
	if output.VMErr != nil {
		t.Fatal("didn't expect error:", output.VMErr)
	}

	num := new(big.Int).SetBytes(output.Value)
	assert.Equal(big.NewInt(11), num)
}

// a contract call another contract.
func TestContractCall(t *testing.T) {
	assert := assert.New(t)

	origin := thor.BytesToAddress([]byte("0x0a"))
	env := newEnv()

	// 创建合约
	var input = common.Hex2Bytes("6060604052341561000f57600080fd5b610017610071565b604051809103906000f080151561002d57600080fd5b6000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff160217905550610081565b6040516101ab806103bf83390190565b61032f806100906000396000f30060606040526004361061004c576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff168063830fb67c1461005157806396f1b6be14610091575b600080fd5b341561005c57600080fd5b61007b60048080359060200190919080359060200190919050506100e6565b6040518082815260200191505060405180910390f35b341561009c57600080fd5b6100a46102de565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b60008060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff166344650b74846040518263ffffffff167c010000000000000000000000000000000000000000000000000000000002815260040180828152602001915050600060405180830381600087803b151561017757600080fd5b6102c65a03f1151561018857600080fd5b5050506000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16639650470c836040518263ffffffff167c010000000000000000000000000000000000000000000000000000000002815260040180828152602001915050600060405180830381600087803b151561021a57600080fd5b6102c65a03f1151561022b57600080fd5b5050506000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff166367e068586000604051602001526040518163ffffffff167c0100000000000000000000000000000000000000000000000000000000028152600401602060405180830381600087803b15156102bb57600080fd5b6102c65a03f115156102cc57600080fd5b50505060405180519050905092915050565b6000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff16815600a165627a7a72305820aa5680aaafffb2c975d04c68f53abed365e4a87aab84366cfd47e244d023188900296060604052341561000f57600080fd5b61018d8061001e6000396000f30060606040526004361061006d576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806316e64048146100725780631f2a63c01461009b57806344650b74146100c457806367e06858146100e75780639650470c14610110575b600080fd5b341561007d57600080fd5b610085610133565b6040518082815260200191505060405180910390f35b34156100a657600080fd5b6100ae610139565b6040518082815260200191505060405180910390f35b34156100cf57600080fd5b6100e5600480803590602001909190505061013f565b005b34156100f257600080fd5b6100fa610149565b6040518082815260200191505060405180910390f35b341561011b57600080fd5b6101316004808035906020019091905050610157565b005b60005481565b60015481565b8060008190555050565b600060015460005401905090565b80600181905550505600a165627a7a72305820509096b2d0bdf296da55f8c6593cd4a375d04f06be748020cec5b9e001d4f8ee0029")
	output := env.Create(origin, input, math.MaxUint64, new(big.Int))
	contractAddr := *output.ContractAddress
	if output.VMErr != nil {
		t.Fatal("didn't expect error:", output.VMErr)
	}

	// 创建 abi
	var definition = `[{"constant":false,"inputs":[{"name":"a","type":"int256"},{"name":"b","type":"int256"}],"name":"Add","outputs":[{"name":"","type":"int256"}],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":true,"inputs":[],"name":"calc","outputs":[{"name":"","type":"address"}],"payable":false,"stateMutability":"view","type":"function"},{"inputs":[],"payable":false,"stateMutability":"nonpayable","type":"constructor"}]`
	abi, err := abi.JSON(strings.NewReader(definition))
	if err != nil {
		t.Fatal(err)
	}

	// ADD
	add, err := abi.Pack("Add", big.NewInt(5), big.NewInt(10))
	if err != nil {
		t.Fatal(err)
	}
	output = env.Call(origin, contractAddr, add, math.MaxUint64, new(big.Int))
	if output.VMErr != nil {
		t.Fatal("didn't expect error:", output.VMErr)
	}

	num := new(big.Int).SetBytes(output.Value)
	assert.Equal(big.NewInt(15), num)
}

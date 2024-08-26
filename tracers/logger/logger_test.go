// Copyright 2021 The go-ethereum Authors
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

package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/vm"
)

type dummyContractRef struct {
	calledForEach bool
}

func (dummyContractRef) Address() common.Address     { return common.Address{} }
func (dummyContractRef) Value() *big.Int             { return new(big.Int) }
func (dummyContractRef) SetCode(common.Hash, []byte) {}
func (d *dummyContractRef) ForEachStorage(callback func(key, value common.Hash) bool) {
	d.calledForEach = true
}
func (d *dummyContractRef) SubBalance(amount *big.Int) {}
func (d *dummyContractRef) AddBalance(amount *big.Int) {}
func (d *dummyContractRef) SetBalance(*big.Int)        {}
func (d *dummyContractRef) SetNonce(uint64)            {}
func (d *dummyContractRef) Balance() *big.Int          { return new(big.Int) }

type dummyStatedb struct {
	vm.StateDB
}

func (*dummyStatedb) GetRefund() uint64                                       { return 1337 }
func (*dummyStatedb) GetState(_ common.Address, _ common.Hash) common.Hash    { return common.Hash{} }
func (*dummyStatedb) SetState(_ common.Address, _ common.Hash, _ common.Hash) {}

func TestStoreCapture(t *testing.T) {
	var (
		unCastedLogger, _ = NewStructLogger(nil)
		env               = vm.NewEVM(vm.Context{}, &dummyStatedb{}, &vm.ChainConfig{ChainConfig: *params.TestChainConfig}, vm.Config{Tracer: unCastedLogger.(*StructLogger)})

		contract = vm.NewContract(&dummyContractRef{}, &dummyContractRef{}, new(big.Int), 100000)
	)
	logger := unCastedLogger.(*StructLogger)
	contract.Code = []byte{byte(vm.PUSH1), 0x1, byte(vm.PUSH1), 0x0, byte(vm.SSTORE)}
	var index common.Hash
	logger.CaptureStart(env, common.Address{}, contract.Address(), false, nil, 0, nil)
	_, err := env.Interpreter().Run(contract, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	if len(logger.storage[contract.Address()]) == 0 {
		t.Fatalf("expected exactly 1 changed value on address %x, got %d", contract.Address(),
			len(logger.storage[contract.Address()]))
	}
	exp := common.BigToHash(big.NewInt(1))
	if logger.storage[contract.Address()][index] != exp {
		t.Errorf("expected %x, got %x", exp, logger.storage[contract.Address()][index])
	}
}

// Tests that blank fields don't appear in logs when JSON marshalled, to reduce
// logs bloat and confusion. See https://github.com/ethereum/go-ethereum/issues/24487
func TestStructLogMarshalingOmitEmpty(t *testing.T) {
	tests := []struct {
		name string
		log  *StructLog
		want string
	}{
		{"empty err and no fields", &StructLog{},
			`{"pc":0,"op":0,"gas":"0x0","gasCost":"0x0","memSize":0,"stack":null,"depth":0,"refund":0,"opName":"STOP"}`},
		{"with err", &StructLog{Err: errors.New("this failed")},
			`{"pc":0,"op":0,"gas":"0x0","gasCost":"0x0","memSize":0,"stack":null,"depth":0,"refund":0,"opName":"STOP","error":"this failed"}`},
		{"with mem", &StructLog{Memory: make([]byte, 2), MemorySize: 2},
			`{"pc":0,"op":0,"gas":"0x0","gasCost":"0x0","memory":"0x0000","memSize":2,"stack":null,"depth":0,"refund":0,"opName":"STOP"}`},
		{"with 0-size mem", &StructLog{Memory: make([]byte, 0)},
			`{"pc":0,"op":0,"gas":"0x0","gasCost":"0x0","memSize":0,"stack":null,"depth":0,"refund":0,"opName":"STOP"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blob, err := json.Marshal(tt.log)
			if err != nil {
				t.Fatal(err)
			}
			if have, want := string(blob), tt.want; have != want {
				t.Fatalf("mismatched results\n\thave: %v\n\twant: %v", have, want)
			}
		})
	}
}

func TestFormatLogs(t *testing.T) {
	logs := []StructLog{
		{Pc: 1, Op: vm.PUSH1, Gas: 100, GasCost: 2, Depth: 1, Memory: []byte("test"), Stack: []uint256.Int{*uint256.NewInt(1)}},
	}

	formattedLogs := formatLogs(logs)
	if len(formattedLogs) != len(logs) {
		t.Errorf("Expected %d formatted logs, got %d", len(logs), len(formattedLogs))
	}
}

func TestCaptureStart(t *testing.T) {
	unCastedLogger, _ := NewStructLogger(nil)
	logger := unCastedLogger.(*StructLogger)
	env := &vm.EVM{}

	logger.CaptureStart(env, common.Address{}, common.Address{}, false, nil, 0, nil)
	logger.CaptureEnd(nil, 1234, fmt.Errorf("Some Error"))
	logger.CaptureClauseEnd(10000)
	logger.Stop(fmt.Errorf("Some Error"))
	logger.CaptureClauseStart(1234)
	logger.CaptureClauseEnd(1234)
	logger.Reset()
}

func TestNewMarkdownLogger(t *testing.T) {
	writer := &bytes.Buffer{}
	cfg := &Config{EnableMemory: true}
	logger := NewMarkdownLogger(cfg, writer)

	if logger.cfg != cfg {
		t.Errorf("Expected cfg to be set correctly")
	}

	env := &vm.EVM{}

	logger.CaptureStart(env, common.Address{}, common.Address{}, false, nil, 0, nil)
	logger.CaptureEnd(nil, 1234, fmt.Errorf("Some Error"))
	logger.CaptureClauseEnd(10000)
	logger.CaptureClauseStart(1234)
	logger.CaptureClauseEnd(1234)
}

func TestWriteLogs(t *testing.T) {
	writer := &bytes.Buffer{}

	logs := []*types.Log{
		{
			Address:     common.HexToAddress("0x1"),
			Topics:      []common.Hash{common.BytesToHash([]byte("topic1")), common.BytesToHash([]byte("topic2"))},
			Data:        []byte("data1"),
			BlockNumber: 100,
			TxHash:      common.BytesToHash([]byte("txhash1")),
			TxIndex:     0,
			BlockHash:   common.BytesToHash([]byte("blockhash1")),
			Index:       0,
			Removed:     false,
		},
		{
			Address:     common.HexToAddress("0x2"),
			Topics:      []common.Hash{common.BytesToHash([]byte("topic3")), common.BytesToHash([]byte("topic4"))},
			Data:        []byte("data2"),
			BlockNumber: 101,
			TxHash:      common.BytesToHash([]byte("txhash2")),
			TxIndex:     1,
			BlockHash:   common.BytesToHash([]byte("blockhash2")),
			Index:       1,
			Removed:     false,
		},
	}

	WriteLogs(writer, logs)
	assert.NotNil(t, writer)
}

func TestWriteTrace(t *testing.T) {
	writer := &bytes.Buffer{}

	logs := []StructLog{
		{
			Pc:            1,
			Op:            vm.PUSH1,
			Gas:           21000,
			GasCost:       3,
			Memory:        []byte("example memory"),
			MemorySize:    len("example memory"),
			Stack:         []uint256.Int{*uint256.NewInt(2)},
			ReturnData:    []byte("return data"),
			Storage:       make(map[common.Hash]common.Hash),
			Depth:         0,
			RefundCounter: 100,
			Err:           nil,
		},
	}

	WriteTrace(writer, logs)
	assert.NotNil(t, writer)
}

func TestGetResult(t *testing.T) {
	logger, _ := NewStructLogger(nil)

	rawMessage, err := logger.GetResult()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	assert.NotNil(t, rawMessage)
}

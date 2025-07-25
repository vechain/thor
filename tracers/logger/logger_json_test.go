// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logger

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/vechain/thor/v2/vm"
)

// Implement the StateDB interface partially for testing
type mockStateDB struct {
	vm.StateDB
	refund uint64
}

func (m *mockStateDB) GetRefund() uint64 {
	return m.refund
}

func TestJSONLogger(t *testing.T) {
	var buf bytes.Buffer

	logger := NewJSONLogger(nil, &buf)
	env := &vm.EVM{}
	stateDB := &mockStateDB{refund: 100}
	env.StateDB = stateDB

	logger.CaptureStart(env, common.Address{}, common.Address{}, false, nil, 0, big.NewInt(0))

	memory := vm.NewMemory()
	stack := &vm.Stack{}
	contract := vm.NewContract(vm.AccountRef(common.Address{}), vm.AccountRef(common.Address{}), big.NewInt(0), 0)

	logger.CaptureState(0, vm.ADD, 10, 2, memory, stack, contract, nil, 1, nil)
	logger.CaptureEnd(nil, 0, nil)

	var logs []json.RawMessage
	decoder := json.NewDecoder(&buf)
	for {
		var log json.RawMessage
		if err := decoder.Decode(&log); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("Failed to decode log: %v", err)
		}
		logs = append(logs, log)
	}

	if len(logs) != 2 {
		t.Errorf("Expected 2 logs, got %d", len(logs))
	}
}

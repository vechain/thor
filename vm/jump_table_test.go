// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

func TestPush0OpCode(t *testing.T) {
	// Arrange
	mockStack := newstack()
	jt := NewShanghaiInstructionSet()
	push0 := jt[PUSH0]

	// Check the stack is empty before executing PUSH0
	assert.Equal(t, 0, mockStack.len())

	// Act
	push0.execute(nil, nil, nil, nil, mockStack)

	// Assert
	// Check the stack has one zero element after executing PUSH0
	assert.Equal(t, 1, mockStack.len())

	expectedValueOnStack := new(uint256.Int)
	assert.Equal(t, expectedValueOnStack, mockStack.peek())
}

func TestMcopyForkGating(t *testing.T) {
	t.Run("MCOPY unavailable pre-Cancun", func(t *testing.T) {
		shanghaiJt := NewShanghaiInstructionSet()
		assert.Nil(t, shanghaiJt[MCOPY], "MCOPY should not exist in Shanghai instruction set")

		istanbulJt := NewIstanbulInstructionSet()
		assert.Nil(t, istanbulJt[MCOPY], "MCOPY should not exist in Istanbul instruction set")
	})

	t.Run("MCOPY available on Cancun", func(t *testing.T) {
		cancunJt := NewCancunInstructionSet()
		assert.NotNil(t, cancunJt[MCOPY], "MCOPY should exist in Cancun instruction set")
		assert.NotNil(t, cancunJt[MCOPY].execute)
		assert.NotNil(t, cancunJt[MCOPY].gasCost)
		assert.NotNil(t, cancunJt[MCOPY].memorySize)
	})
}

func TestSELFDESTRUCTForkGating(t *testing.T) {
	t.Run("SELFDESTRUCT unavailable pre-Cancun", func(t *testing.T) {
		shanghaiJt := NewShanghaiInstructionSet()

		funcName := runtime.FuncForPC(reflect.ValueOf(shanghaiJt[SELFDESTRUCT].execute).Pointer()).Name()
		assert.NotNil(t, shanghaiJt[SELFDESTRUCT], "SELFDESTRUCT should exist in Shanghai instruction set")
		assert.NotNil(t, shanghaiJt[SELFDESTRUCT].gasCost)
		assert.NotNil(t, shanghaiJt[SELFDESTRUCT].validateStack)
		assert.Equal(t, shanghaiJt[SELFDESTRUCT].halts, true)
		assert.Equal(t, shanghaiJt[SELFDESTRUCT].writes, true)

		assert.Equal(t, funcName, "github.com/vechain/thor/v2/vm.opSuicide", "SELFDESTRUCT should be implemented as opSuicide in Shanghai instruction set")
	})

	t.Run("SELFDESTRUCT available on Cancun", func(t *testing.T) {
		cancunJt := NewCancunInstructionSet()

		funcName := runtime.FuncForPC(reflect.ValueOf(cancunJt[SELFDESTRUCT].execute).Pointer()).Name()
		assert.NotNil(t, cancunJt[SELFDESTRUCT], "SELFDESTRUCT should exist in Cancun instruction set")
		assert.NotNil(t, cancunJt[SELFDESTRUCT].gasCost)
		assert.NotNil(t, cancunJt[SELFDESTRUCT].validateStack)
		assert.Equal(t, cancunJt[SELFDESTRUCT].halts, true)
		assert.Equal(t, cancunJt[SELFDESTRUCT].writes, true)

		assert.Equal(t, funcName, "github.com/vechain/thor/v2/vm.opSuicide6780", "SELFDESTRUCT should be implemented as opSuicide in Cancun instruction set")
	})
}

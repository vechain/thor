// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
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

	t.Run("CLZ unavailable pre-Osaka", func(t *testing.T) {
		shanghaiJt := NewShanghaiInstructionSet()
		assert.Nil(t, shanghaiJt[CLZ], "CLZ should not exist in Shanghai instruction set")

		istanbulJt := NewIstanbulInstructionSet()
		assert.Nil(t, istanbulJt[CLZ], "CLZ should not exist in Istanbul instruction set")

		cancunJt := NewCancunInstructionSet()
		assert.Nil(t, cancunJt[CLZ], "CLZ should not exist in Cancun instruction set")
	})

	t.Run("CLZ available on Osaka", func(t *testing.T) {
		osakaJt := NewOsakaInstructionSet()
		assert.NotNil(t, osakaJt[CLZ], "CLZ should exist in Osaka instruction set")
		assert.NotNil(t, osakaJt[CLZ].execute)
		assert.NotNil(t, osakaJt[CLZ].gasCost)
	})
}

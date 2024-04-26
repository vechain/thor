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

// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

func TestStackPushPop(t *testing.T) {
	stack := newstack()
	defer returnStack(stack)

	val := uint256.NewInt(42)
	stack.push(val)

	assert.Equal(t, 1, stack.len(), "Stack should have one item after push")
	assert.Equal(t, val, stack.peek(), "Top item should be the one that was pushed")

	popped := stack.pop()
	assert.Equal(t, val, &popped, "Popped item should be equal to pushed item")
	assert.Equal(t, 0, stack.len(), "Stack should be empty after pop")
}

func TestStackSwap(t *testing.T) {
	stack := newstack()
	defer returnStack(stack)

	first := uint256.NewInt(1)
	second := uint256.NewInt(2)
	stack.push(first)
	stack.push(second)

	stack.swap(2)
	assert.Equal(t, first, stack.peek(), "Top item should be the first one after swap")
}

func TestStackDup(t *testing.T) {
	stack := newstack()
	defer returnStack(stack)

	val := uint256.NewInt(42)
	stack.push(val)
	stack.dup(1)

	assert.Equal(t, 2, stack.len(), "Stack should have two items after dup")
	assert.Equal(t, val, stack.peek(), "Top item should be the same as the duplicated one")
}

func TestStackBack(t *testing.T) {
	stack := newstack()
	defer returnStack(stack)

	first := uint256.NewInt(1)
	second := uint256.NewInt(2)
	stack.push(first)
	stack.push(second)

	back := stack.Back(1)
	assert.Equal(t, first, back, "Back should return the first item")
}

func TestStackPrint(t *testing.T) {
	stack := newstack()
	defer returnStack(stack)

	// Test printing an empty and a non-empty stack
	// Since Print() doesn't return anything, we are just
	// making sure it doesn't crash for now
	stack.Print()

	val := uint256.NewInt(42)
	stack.push(val)
	stack.Print()
}

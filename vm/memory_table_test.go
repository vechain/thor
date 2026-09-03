// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"math"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

func mockStack(data ...int64) *Stack {
	stack := newstack()

	for _, item := range data {
		// Convert int64 to uint256 and push onto the stack
		stack.push(uint256.NewInt(uint64(item)))
	}

	return stack
}

// Test for memorySha3 function
func TestMemorySha3(t *testing.T) {
	tests := []struct {
		name      string
		stackData []int64
		expected  uint64
		overflow  bool
	}{
		{
			name:      "Normal case",
			stackData: []int64{10, 32},
			expected:  42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := mockStack(tt.stackData...)
			got, _ := memorySha3(stack)
			if got != tt.expected {
				t.Errorf("memorySha3() got = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Test for memoryCallDataCopy function
func TestMemoryCallDataCopy(t *testing.T) {
	tests := []struct {
		name      string
		stackData []int64
		expected  uint64
	}{
		{
			name:      "Normal case",
			stackData: []int64{0, 10, 32}, // Position 0, Size 32
			expected:  0,
		},
		{
			name:      "Overflow case",
			stackData: []int64{0, math.MaxInt64, 1},
			expected:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := mockStack(tt.stackData...)
			got, _ := memoryCallDataCopy(stack)
			if got != tt.expected {
				t.Errorf("memoryCallDataCopy() got = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Test for memoryReturnDataCopy function
func TestMemoryReturnDataCopy(t *testing.T) {
	tests := []struct {
		name      string
		stackData []int64
		expected  uint64
	}{
		{
			name:      "Normal case",
			stackData: []int64{0, 10, 32}, // Position 0, Size 32
			expected:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := mockStack(tt.stackData...)
			got, _ := memoryReturnDataCopy(stack)
			if got != tt.expected {
				t.Errorf("memoryReturnDataCopy() got = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Test for memoryCodeCopy function
func TestMemoryCodeCopy(t *testing.T) {
	tests := []struct {
		name      string
		stackData []int64
		expected  uint64
	}{
		{
			name:      "Normal case",
			stackData: []int64{0, 10, 32}, // Position 0, Size 32
			expected:  0,
		},
		// Additional test cases can be added here...
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := mockStack(tt.stackData...)
			got, _ := memoryCodeCopy(stack)
			if got != tt.expected {
				t.Errorf("memoryCodeCopy() got = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Test for memoryExtCodeCopy function
func TestMemoryExtCodeCopy(t *testing.T) {
	stack := mockStack(0, 10, 0, 32) // Example stack data
	expected := uint64(0)            // Replace with expected value
	got, _ := memoryExtCodeCopy(stack)
	if got != expected {
		t.Errorf("memoryExtCodeCopy() got = %v, want %v", got, expected)
	}
}

// Test for memoryMLoad function
func TestMemoryMLoad(t *testing.T) {
	stack := mockStack(10) // Example stack data
	expected := uint64(42) // Replace with expected value
	got, _ := memoryMLoad(stack)
	if got != expected {
		t.Errorf("memoryMLoad() got = %v, want %v", got, expected)
	}
}

// Test for memoryMStore8 function
func TestMemoryMStore8(t *testing.T) {
	stack := mockStack(10) // Example stack data
	expected := uint64(11) // Replace with expected value
	got, _ := memoryMStore8(stack)
	if got != expected {
		t.Errorf("memoryMStore8() got = %v, want %v", got, expected)
	}
}

// Test for memoryMStore function
func TestMemoryMStore(t *testing.T) {
	stack := mockStack(10) // Example stack data
	expected := uint64(42) // Replace with expected value
	got, _ := memoryMStore(stack)
	if got != expected {
		t.Errorf("memoryMStore() got = %v, want %v", got, expected)
	}
}

// Test for memoryCreate function
func TestMemoryCreate(t *testing.T) {
	stack := mockStack(0, 10, 32) // Example stack data
	expected := uint64(0)         // Replace with expected value
	got, _ := memoryCreate(stack)
	if got != expected {
		t.Errorf("memoryCreate() got = %v, want %v", got, expected)
	}
}

// Test for memoryCreate2 function
func TestMemoryCreate2(t *testing.T) {
	stack := mockStack(0, 10, 32) // Example stack data
	expected := uint64(0)         // Replace with expected value
	got, _ := memoryCreate2(stack)
	if got != expected {
		t.Errorf("memoryCreate2() got = %v, want %v", got, expected)
	}
}

// Test for memoryCall function
func TestMemoryCall(t *testing.T) {
	stack := mockStack(0, 0, 0, 10, 0, 0, 32) // Example stack data
	expected := uint64(0)                     // Replace with expected value
	got, _ := memoryCall(stack)
	if got != expected {
		t.Errorf("memoryCall() got = %v, want %v", got, expected)
	}
}

func TestMemoryCallOverflow(t *testing.T) {
	stack := mockStack() // Example stack data
	stack.push(uint256.NewInt(uint64(math.MaxUint64)))
	stack.push(uint256.NewInt(uint64(math.MaxUint64)))
	stack.push(uint256.NewInt(uint64(math.MaxUint64)))
	stack.push(uint256.NewInt(uint64(math.MaxUint64)))
	stack.push(uint256.NewInt(uint64(math.MaxUint64)))
	stack.push(uint256.NewInt(uint64(math.MaxUint64)))
	stack.push(uint256.NewInt(uint64(math.MaxUint64)))

	_, overflow := memoryCall(stack)

	assert.True(t, overflow)
}

// Test for memoryDelegateCall function
func TestMemoryDelegateCall(t *testing.T) {
	stack := mockStack(0, 0, 0, 10, 0, 32) // Example stack data
	expected := uint64(0)                  // Replace with expected value
	got, _ := memoryDelegateCall(stack)
	if got != expected {
		t.Errorf("memoryDelegateCall() got = %v, want %v", got, expected)
	}
}

// Test for memoryStaticCall function
func TestMemoryStaticCall(t *testing.T) {
	stack := mockStack(0, 0, 0, 10, 0, 32) // Example stack data
	expected := uint64(0)                  // Replace with expected value
	got, _ := memoryStaticCall(stack)
	if got != expected {
		t.Errorf("memoryStaticCall() got = %v, want %v", got, expected)
	}
}

// Test for memoryReturn function
func TestMemoryReturn(t *testing.T) {
	stack := mockStack(10, 32) // Example stack data
	expected := uint64(42)     // Replace with expected value
	got, _ := memoryReturn(stack)
	if got != expected {
		t.Errorf("memoryReturn() got = %v, want %v", got, expected)
	}
}

// Test for memoryRevert function
func TestMemoryRevert(t *testing.T) {
	stack := mockStack(10, 32) // Example stack data
	expected := uint64(42)     // Replace with expected value
	got, _ := memoryRevert(stack)
	if got != expected {
		t.Errorf("memoryRevert() got = %v, want %v", got, expected)
	}
}

// Test for memoryMcopy function
func TestMemoryMcopy(t *testing.T) {
	tests := []struct {
		name      string
		stackData []int64
		expected  uint64
		overflow  bool
	}{
		{
			name:      "dst > src",
			stackData: []int64{32, 32, 0},
			expected:  64,
		},
		{
			name:      "src > dst",
			stackData: []int64{32, 0, 32},
			expected:  64,
		},
		{
			name:      "same offset",
			stackData: []int64{32, 0, 0},
			expected:  32,
		},
		{
			name:      "zero length",
			stackData: []int64{0, 10, 20},
			expected:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := mockStack(tt.stackData...)
			got, overflow := memoryMcopy(stack)
			assert.Equal(t, tt.overflow, overflow)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestMemoryMcopyOverflow(t *testing.T) {
	t.Run("length exceeds uint64", func(t *testing.T) {
		stack := newstack()
		stack.push(new(uint256.Int).Lsh(uint256.NewInt(1), 128)) // length = 2^128
		stack.push(uint256.NewInt(0))                            // src = 0
		stack.push(uint256.NewInt(0))                            // dst = 0

		_, overflow := memoryMcopy(stack)
		assert.True(t, overflow)
	})

	t.Run("dst exceeds uint64", func(t *testing.T) {
		stack := newstack()
		stack.push(uint256.NewInt(1))                            // length = 1
		stack.push(uint256.NewInt(0))                            // src = 0
		stack.push(new(uint256.Int).Lsh(uint256.NewInt(1), 128)) // dst = 2^128

		_, overflow := memoryMcopy(stack)
		assert.True(t, overflow)
	})

	t.Run("src exceeds uint64", func(t *testing.T) {
		stack := newstack()
		stack.push(uint256.NewInt(1))                            // length = 1
		stack.push(new(uint256.Int).Lsh(uint256.NewInt(1), 128)) // src = 2^128
		stack.push(uint256.NewInt(0))                            // dst = 0

		_, overflow := memoryMcopy(stack)
		assert.True(t, overflow)
	})

	t.Run("offset plus length overflows uint64", func(t *testing.T) {
		stack := newstack()
		stack.push(uint256.NewInt(1))                          // length = 1
		stack.push(uint256.NewInt(0))                          // src = 0
		stack.push(new(uint256.Int).SetUint64(math.MaxUint64)) // dst = MaxUint64

		_, overflow := memoryMcopy(stack)
		assert.True(t, overflow)
	})
}

// Test for memoryLog function
func TestMemoryLog(t *testing.T) {
	stack := mockStack(10, 32) // Example stack data
	expected := uint64(42)     // Replace with expected value
	got, _ := memoryLog(stack)
	if got != expected {
		t.Errorf("memoryLog() got = %v, want %v", got, expected)
	}
}

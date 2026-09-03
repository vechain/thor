// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"testing"
)

// TestOpCodeString tests the String method of OpCode.
func TestOpCodeString(t *testing.T) {
	tests := []struct {
		op       OpCode
		expected string
	}{
		{STOP, "STOP"},
		{ADD, "ADD"},
		{MUL, "MUL"},
		// ... add more tests for different opcodes
	}

	for _, tt := range tests {
		actual := tt.op.String()
		if actual != tt.expected {
			t.Errorf("OpCode.String() for %v: expected %s, got %s", tt.op, tt.expected, actual)
		}
	}
}

// TestStringToOp tests the StringToOp function.
func TestStringToOp(t *testing.T) {
	tests := []struct {
		name     string
		expected OpCode
	}{
		{"STOP", STOP},
		{"ADD", ADD},
		{"MUL", MUL},
		// ... add more tests for different opcode names
	}

	for _, tt := range tests {
		actual := StringToOp(tt.name)
		if actual != tt.expected {
			t.Errorf("StringToOp(%s): expected %v, got %v", tt.name, tt.expected, actual)
		}
	}
}

// TestOpCodeIsPush tests the IsPush method of OpCode.
func TestOpCodeIsPush(t *testing.T) {
	if !PUSH1.IsPush() {
		t.Errorf("PUSH1 should be a push operation")
	}
	if STOP.IsPush() {
		t.Errorf("STOP should not be a push operation")
	}
	// ... add more tests for different opcodes
}

// TestOpCodeIsStaticJump tests the IsStaticJump method of OpCode.
func TestOpCodeIsStaticJump(t *testing.T) {
	if !JUMP.IsStaticJump() {
		t.Errorf("JUMP should be a static jump operation")
	}
	if ADD.IsStaticJump() {
		t.Errorf("ADD should not be a static jump operation")
	}
	// ... add more tests for different opcodes
}

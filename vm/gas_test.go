// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vm

import (
	"testing"

	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

func TestCallGas(t *testing.T) {
	gasTable := params.GasTableEIP150

	// Define test cases
	tests := []struct {
		name           string
		availableGas   uint64
		base           uint64
		callCost       *uint256.Int
		expectedGas    uint64
		expectingError bool
	}{
		{
			name:           "Basic Calculation",
			availableGas:   1000,
			base:           200,
			callCost:       uint256.NewInt(500),
			expectedGas:    500,
			expectingError: false,
		},
		{
			name:           "Invalid Gas",
			availableGas:   1000,
			base:           200,
			callCost:       uint256.NewInt(351238154142),
			expectedGas:    788,
			expectingError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := callGas(gasTable, tt.availableGas, tt.base, tt.callCost)
			if (err != nil) != tt.expectingError {
				t.Errorf("callGas() error = %v, expectingError %v", err, tt.expectingError)
				return
			}
			if got != tt.expectedGas {
				t.Errorf("callGas() = %v, want %v", got, tt.expectedGas)
			}
		})
	}
}

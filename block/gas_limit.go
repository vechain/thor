// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"math"

	"github.com/vechain/thor/thor"
)

// GasLimit to support block gas limit validation and adjustment.
type GasLimit uint64

// IsValid returns if the receiver is valid according to parent gas limit.
func (gl GasLimit) IsValid(parentGasLimit uint64) bool {
	gasLimit := uint64(gl)
	if gasLimit < thor.MinGasLimit {
		return false
	}
	var diff uint64
	if gasLimit > parentGasLimit {
		diff = gasLimit - parentGasLimit
	} else {
		diff = parentGasLimit - gasLimit
	}

	return diff <= parentGasLimit/thor.GasLimitBoundDivisor
}

// Qualify qualify the receiver according to parent gas limit, and returns
// the qualified gas limit value.
func (gl GasLimit) Qualify(parentGasLimit uint64) uint64 {
	gasLimit := uint64(gl)
	maxDiff := parentGasLimit / thor.GasLimitBoundDivisor
	if gasLimit > parentGasLimit {
		diff := min64(gasLimit-parentGasLimit, maxDiff)
		return GasLimit(parentGasLimit).Adjust(int64(diff))
	}
	diff := min64(parentGasLimit-gasLimit, maxDiff)
	return GasLimit(parentGasLimit).Adjust(-int64(diff))
}

// Adjust suppose the receiver is parent gas limit, and calculate a valid
// gas limit value by apply `delta`.
func (gl GasLimit) Adjust(delta int64) uint64 {
	gasLimit := uint64(gl)
	maxDiff := gasLimit / thor.GasLimitBoundDivisor

	if delta > 0 {
		// increase
		diff := min64(uint64(delta), maxDiff)
		if math.MaxUint64-diff < gasLimit {
			// overflow case
			return math.MaxUint64
		}
		return gasLimit + diff
	}

	// reduce
	diff := min64(uint64(-delta), maxDiff)
	if thor.MinGasLimit+diff > gasLimit {
		// reach floor
		return thor.MinGasLimit
	}
	return gasLimit - diff
}

func min64(a, b uint64) uint64 {
	if a > b {
		return b
	}
	return a
}

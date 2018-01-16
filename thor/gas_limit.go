package thor

import (
	"math"
)

// GasLimit to support block gas limit validation and adjustment.
type GasLimit uint64

// IsValid returns if the receiver is valid according to parent gas limit.
func (gl GasLimit) IsValid(parentGasLimit uint64) bool {
	gasLimit := uint64(gl)
	if gasLimit < MinGasLimit {
		return false
	}
	var diff uint64
	if gasLimit > parentGasLimit {
		diff = gasLimit - parentGasLimit
	} else {
		diff = parentGasLimit - gasLimit
	}

	return diff <= parentGasLimit/GasLimitBoundDivisor
}

// Adjust suppose the receiver is parent gas limit, and calculate a valid
// gas limit value by apply `delta`.
func (gl GasLimit) Adjust(delta int64) uint64 {
	gasLimit := uint64(gl)
	maxDiff := gasLimit / GasLimitBoundDivisor

	if delta > 0 {
		// increase

		diff := uint64(delta)
		if diff > maxDiff {
			diff = maxDiff
		}
		if math.MaxUint64-diff < gasLimit {
			// overflow case
			return math.MaxUint64
		}
		return gasLimit + diff
	}

	// reduce
	diff := uint64(-delta)
	if diff > maxDiff {
		diff = maxDiff
	}
	if MinGasLimit+diff > gasLimit {
		// reach floor
		return MinGasLimit
	}
	return gasLimit - diff
}

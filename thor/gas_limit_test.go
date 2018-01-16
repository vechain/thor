package thor_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/thor"
)

func TestGasLimit_IsValid(t *testing.T) {

	tests := []struct {
		gl       uint64
		parentGL uint64
		want     bool
	}{
		{thor.MinGasLimit, thor.MinGasLimit, true},
		{thor.MinGasLimit - 1, thor.MinGasLimit, false},
		{thor.MinGasLimit, thor.MinGasLimit * 2, false},
		{thor.MinGasLimit * 2, thor.MinGasLimit, false},
		{thor.MinGasLimit + thor.MinGasLimit/thor.GasLimitBoundDivisor, thor.MinGasLimit, true},
		{thor.MinGasLimit*2 + thor.MinGasLimit/thor.GasLimitBoundDivisor, thor.MinGasLimit * 2, true},
		{thor.MinGasLimit*2 - thor.MinGasLimit/thor.GasLimitBoundDivisor, thor.MinGasLimit * 2, true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, thor.GasLimit(tt.gl).IsValid(tt.parentGL))
	}
}

func TestGasLimit_Adjust(t *testing.T) {

	tests := []struct {
		gl    uint64
		delta int64
		want  uint64
	}{
		{thor.MinGasLimit, 1, thor.MinGasLimit + 1},
		{thor.MinGasLimit, -1, thor.MinGasLimit},
		{math.MaxUint64, 1, math.MaxUint64},
		{thor.MinGasLimit, int64(thor.MinGasLimit), thor.MinGasLimit + thor.MinGasLimit/thor.GasLimitBoundDivisor},
		{thor.MinGasLimit * 2, -int64(thor.MinGasLimit), thor.MinGasLimit*2 - (thor.MinGasLimit*2)/thor.GasLimitBoundDivisor},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, thor.GasLimit(tt.gl).Adjust(tt.delta))
	}
}

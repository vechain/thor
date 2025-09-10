package thor

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockParams implements the interface required by GetMaxBlockProposers
type mockParams struct {
	values map[Bytes32]*big.Int
}

func (m *mockParams) Get(key Bytes32) (*big.Int, error) {
	if val, exists := m.values[key]; exists {
		return val, nil
	}
	return big.NewInt(0), nil
}

func TestGetMaxBlockProposers(t *testing.T) {
	t.Run("returns initial value when no parameter is set", func(t *testing.T) {
		params := &mockParams{values: make(map[Bytes32]*big.Int)}

		maxBlockProposers, err := GetMaxBlockProposers(params, false)
		assert.NoError(t, err)
		assert.Equal(t, uint64(101), maxBlockProposers) // InitialMaxBlockProposers
	})

	t.Run("returns initial value when parameter is zero", func(t *testing.T) {
		params := &mockParams{
			values: map[Bytes32]*big.Int{
				KeyMaxBlockProposers: big.NewInt(0),
			},
		}

		maxBlockProposers, err := GetMaxBlockProposers(params, false)
		assert.NoError(t, err)
		assert.Equal(t, uint64(101), maxBlockProposers) // InitialMaxBlockProposers
	})

	t.Run("returns parameter value when set and not capped", func(t *testing.T) {
		params := &mockParams{
			values: map[Bytes32]*big.Int{
				KeyMaxBlockProposers: big.NewInt(50),
			},
		}

		maxBlockProposers, err := GetMaxBlockProposers(params, false)
		assert.NoError(t, err)
		assert.Equal(t, uint64(50), maxBlockProposers)
	})

	t.Run("caps to initial value when capToInitial is true and value exceeds initial", func(t *testing.T) {
		params := &mockParams{
			values: map[Bytes32]*big.Int{
				KeyMaxBlockProposers: big.NewInt(200), // Greater than InitialMaxBlockProposers (101)
			},
		}

		maxBlockProposers, err := GetMaxBlockProposers(params, true)
		assert.NoError(t, err)
		assert.Equal(t, uint64(101), maxBlockProposers) // Capped to InitialMaxBlockProposers
	})

	t.Run("does not cap when capToInitial is false even if value exceeds initial", func(t *testing.T) {
		params := &mockParams{
			values: map[Bytes32]*big.Int{
				KeyMaxBlockProposers: big.NewInt(200), // Greater than InitialMaxBlockProposers (101)
			},
		}

		maxBlockProposers, err := GetMaxBlockProposers(params, false)
		assert.NoError(t, err)
		assert.Equal(t, uint64(200), maxBlockProposers) // Not capped
	})

	t.Run("returns parameter value when set and within initial limit with capToInitial true", func(t *testing.T) {
		params := &mockParams{
			values: map[Bytes32]*big.Int{
				KeyMaxBlockProposers: big.NewInt(50), // Less than InitialMaxBlockProposers (101)
			},
		}

		maxBlockProposers, err := GetMaxBlockProposers(params, true)
		assert.NoError(t, err)
		assert.Equal(t, uint64(50), maxBlockProposers)
	})
}

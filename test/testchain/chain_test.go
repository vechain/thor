package testchain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ChainDefault(t *testing.T) {
	chain, err := NewDefault()
	require.NoError(t, err)

	for range 1000 {
		require.NoError(t, chain.MintBlock())
	}

	best := chain.Repo().BestBlockSummary()
	require.Equal(t, uint32(1000), best.Header.Number())
}

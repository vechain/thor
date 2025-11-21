// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

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

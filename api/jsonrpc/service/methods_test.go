// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package service

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/test/testchain"
)

func newTestBackend(t *testing.T) *Backend {
	tc, err := testchain.NewDefault()
	require.NoError(t, err)
	return NewBackend(tc.Repo())
}

func TestExampleMethods(t *testing.T) {
	b := newTestBackend(t)

	// genesis-only chain: best block number is 0
	num, err := NewEth(b).BlockNumber()
	require.NoError(t, err)
	assert.Equal(t, hexutil.Uint64(0), num)

	// chainId and net_version are deterministic per genesis but value not asserted; must not error
	chainID, err := NewEth(b).ChainId()
	require.NoError(t, err)
	require.NotNil(t, chainID)

	version := NewNet(b).Version()
	require.NotEmpty(t, version)
}

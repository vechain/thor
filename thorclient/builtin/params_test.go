// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/bind"
)

func TestParams(t *testing.T) {
	executor := bind.NewSigner(genesis.DevAccounts()[0].PrivateKey)

	testNode, client := newTestNode(t, false)
	defer testNode.Stop()

	params, err := NewParams(client)
	require.NoError(t, err)

	t.Run("Get", func(t *testing.T) {
		mbp, err := params.Get(thor.KeyExecutorAddress)
		require.NoError(t, err)
		addr := thor.BytesToAddress(mbp.Bytes())
		require.Equal(t, genesis.DevAccounts()[0].Address, addr)
	})

	t.Run("Set", func(t *testing.T) {
		receipt, _, err := params.Set(thor.KeyMaxBlockProposers, big.NewInt(2)).Send().WithSigner(executor).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
		require.NoError(t, err)
		require.False(t, receipt.Reverted)

		mbp, err := params.Get(thor.KeyMaxBlockProposers)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(2), mbp)

		events, err := params.FilterSet(newRange(receipt), nil, logdb.ASC)
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, thor.KeyMaxBlockProposers, events[0].Key)
		require.Equal(t, big.NewInt(2), events[0].Value)
	})
}

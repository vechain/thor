// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/bind"
)

func TestParams(t *testing.T) {
	executor := (*bind.PrivateKeySigner)(genesis.DevAccounts()[0].PrivateKey)

	_, client := newChain(t, false)

	params, err := NewParams(client)
	require.NoError(t, err)

	t.Run("Get", func(t *testing.T) {
		mbp, err := params.Get(thor.KeyExecutorAddress)
		require.NoError(t, err)
		addr := thor.BytesToAddress(mbp.Bytes())
		require.Equal(t, genesis.DevAccounts()[0].Address, addr)
	})

	t.Run("Set", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		receipt, _, err := params.Set(executor, thor.KeyMaxBlockProposers, big.NewInt(2)).Receipt(ctx, &bind.Options{})
		require.NoError(t, err)
		require.False(t, receipt.Reverted)

		mbp, err := params.Get(thor.KeyMaxBlockProposers)
		require.NoError(t, err)
		require.Equal(t, big.NewInt(2), mbp)

		events, err := params.FilterSet(newRange(receipt), nil, logdb.ASC)
		require.NoError(t, err)
		require.Len(t, events, 1)
	})
}

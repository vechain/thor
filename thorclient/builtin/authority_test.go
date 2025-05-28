// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thorclient/bind"
)

func TestAuthority(t *testing.T) {
	executor := (*bind.PrivateKeySigner)(genesis.DevAccounts()[0].PrivateKey)

	_, client, cancel := newChain(t)
	t.Cleanup(cancel)

	authority, err := NewAuthority(client)
	require.NoError(t, err)

	// Add a new authority for the test
	acc2 := genesis.DevAccounts()[1]
	identity := datagen.RandomHash()

	receipt, _, err := authority.Add(executor, acc2.Address, acc2.Address, identity).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	t.Run("Executor", func(t *testing.T) {
		exec, err := authority.Executor()
		require.NoError(t, err)
		require.Equal(t, executor.Address(), exec)
	})

	t.Run("First", func(t *testing.T) {
		first, err := authority.First()
		require.NoError(t, err)
		require.Equal(t, genesis.DevAccounts()[0].Address, first)
	})

	t.Run("Next", func(t *testing.T) {
		first, err := authority.First()
		require.NoError(t, err)

		next, err := authority.Next(first)
		require.NoError(t, err)
		require.Equal(t, acc2.Address, next)
	})

	t.Run("Get", func(t *testing.T) {
		node, err := authority.Get(acc2.Address)
		require.NoError(t, err)
		require.Equal(t, acc2.Address, node.Endorsor)
		require.Equal(t, identity, node.Identity)
		require.True(t, node.Listed)
	})

	t.Run("Revoke", func(t *testing.T) {
		receipt, _, err = authority.Revoke(executor, acc2.Address).Receipt(txContext(t), txOpts())
		require.NoError(t, err)
		require.False(t, receipt.Reverted)

		node, err := authority.Get(acc2.Address)
		require.NoError(t, err)
		require.Equal(t, false, node.Listed)
	})

	// 1 for Add, 1 for Revoke
	events, err := authority.FilterCandidate(nil, nil, logdb.ASC)
	require.NoError(t, err)
	assert.Equal(t, 2, len(events), "Expected two candidate event")
}

package builtin

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thorclient/bind"
)

func TestAuthority(t *testing.T) {
	executor := (*bind.PrivateKeySigner)(genesis.DevAccounts()[0].PrivateKey)

	authority, err := NewAuthority(client)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Add a new authority for the test
	acc2 := genesis.DevAccounts()[1]
	identity := datagen.RandomHash()

	receipt, _, err := authority.Add(executor, acc2.Address, acc2.Address, identity).Receipt(ctx, &bind.Options{})
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
		receipt, _, err = authority.Revoke(executor, acc2.Address).Receipt(ctx, &bind.Options{})
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

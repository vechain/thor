// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contracts "github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thorclient/bind"
)

func TestAuthority(t *testing.T) {
	executor := bind.NewSigner(genesis.DevAccounts()[0].PrivateKey)

	testNode, client := newTestNode(t, false)
	defer testNode.Stop()

	authority, err := NewAuthority(client)
	require.NoError(t, err)

	// Add a new authority for the test
	acc2 := genesis.DevAccounts()[1]
	identity := datagen.RandomHash()

	receipt, _, err := authority.Add(acc2.Address, acc2.Address, identity).
		Send().
		WithSigner(executor).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
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
		receipt, _, err = authority.Revoke(acc2.Address).Send().WithSigner(executor).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
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

func TestAuthority_RawContract(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	authority, err := NewAuthority(client)
	require.NoError(t, err)

	raw := authority.Raw()
	require.NotNil(t, raw)

	require.Equal(t, contracts.Authority.Address, *raw.Address())
	// sanity check that ABI exposes known method
	_, ok := raw.ABI().Methods["first"]
	require.True(t, ok, "expected method 'first' in ABI")
}

func TestAuthority_Revision(t *testing.T) {
	executor := bind.NewSigner(genesis.DevAccounts()[0].PrivateKey)

	node, client := newTestNode(t, false)
	defer node.Stop()

	auth, err := NewAuthority(client)
	require.NoError(t, err)

	acc := genesis.DevAccounts()[1]
	identity := datagen.RandomHash()

	nodeBefore, err := auth.Revision("0").Get(acc.Address)
	require.NoError(t, err)
	require.False(t, nodeBefore.Listed)

	receiptAdd, _, err := auth.Add(acc.Address, acc.Address, identity).
		Send().
		WithSigner(executor).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receiptAdd.Reverted)

	blockAdd := uint64(receiptAdd.Meta.BlockNumber)
	nodeAtAdd, err := auth.Revision(strconv.FormatUint(blockAdd, 10)).Get(acc.Address)
	require.NoError(t, err)
	require.True(t, nodeAtAdd.Listed)

	receiptRevoke, _, err := auth.Revoke(acc.Address).
		Send().
		WithSigner(executor).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receiptRevoke.Reverted)

	blockRevoke := uint64(receiptRevoke.Meta.BlockNumber)
	nodeAtRevoke, err := auth.Revision(strconv.FormatUint(blockRevoke, 10)).Get(acc.Address)
	require.NoError(t, err)
	require.False(t, nodeAtRevoke.Listed)
}

func TestAuthority_FilterCandidate_EventNotFound(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	// Build an Authority wrapper with an ABI that does NOT define the 'Candidate' event
	badContract, err := bind.NewContract(client, contracts.Energy.RawABI(), &contracts.Authority.Address)
	require.NoError(t, err)
	bad := &Authority{contract: badContract}

	_, err = bad.FilterCandidate(nil, nil, logdb.ASC)
	require.Error(t, err)
}

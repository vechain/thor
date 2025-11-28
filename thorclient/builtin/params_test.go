// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	contracts "github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
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

		events, err := params.FilterSet(newRange(receipt))
		require.NoError(t, err)
		require.Len(t, events, 1)
		require.Equal(t, thor.KeyMaxBlockProposers, events[0].Key)
		require.Equal(t, big.NewInt(2), events[0].Value)
	})
}

func TestParams_RawContract(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	p, err := NewParams(client)
	require.NoError(t, err)

	raw := p.Raw()
	require.NotNil(t, raw)
	require.Equal(t, contracts.Params.Address, *raw.Address())
	_, ok := raw.ABI().Methods["get"]
	require.True(t, ok, "expected method 'get' in ABI")
}

func TestParams_Revision(t *testing.T) {
	executor := bind.NewSigner(genesis.DevAccounts()[0].PrivateKey)

	node, client := newTestNode(t, false)
	defer node.Stop()

	p, err := NewParams(client)
	require.NoError(t, err)

	valBefore, err := p.Revision("0").Get(thor.KeyMaxBlockProposers)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(1000), valBefore)

	receipt, _, err := p.Set(thor.KeyMaxBlockProposers, big.NewInt(2)).
		Send().
		WithSigner(executor).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	blockSet := uint64(receipt.Meta.BlockNumber)
	valAtSet, err := p.Revision(strconv.FormatUint(blockSet, 10)).Get(thor.KeyMaxBlockProposers)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(2), valAtSet)

	// Previous block should still see the old value
	prev := strconv.FormatUint(blockSet-1, 10)
	valPrev, err := p.Revision(prev).Get(thor.KeyMaxBlockProposers)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(1000), valPrev)
}

func TestParams_FilterSet_EventNotFound(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	// Use an ABI without the 'Set' event
	badContract, err := bind.NewContract(client, contracts.Energy.RawABI(), &contracts.Params.Address)
	require.NoError(t, err)
	bad := &Params{contract: badContract}

	_, err = bad.FilterSet()
	require.Error(t, err)
}

func TestParams_NegativeMatrix(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "WrongABI_Get",
			run: func(t *testing.T) {
				badContract, err := bind.NewContract(client, contracts.Energy.RawABI(), &contracts.Params.Address)
				require.NoError(t, err)
				bad := &Params{contract: badContract}
				_, err = bad.Get(thor.KeyMaxBlockProposers)
				require.Error(t, err)
			},
		},
		{
			name: "WrongABI_SetClause",
			run: func(t *testing.T) {
				badContract, err := bind.NewContract(client, contracts.Energy.RawABI(), &contracts.Params.Address)
				require.NoError(t, err)
				bad := &Params{contract: badContract}
				_, err = bad.Set(thor.KeyMaxBlockProposers, big.NewInt(1)).Clause()
				require.Error(t, err)
			},
		},
		{
			name: "BadRevision_Get",
			run: func(t *testing.T) {
				p, err := NewParams(client)
				require.NoError(t, err)
				_, err = p.Revision("bad-revision").Get(thor.KeyExecutorAddress)
				require.Error(t, err)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}

func TestParams_SetClause_PackError(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	p, err := NewParams(client)
	require.NoError(t, err)

	var nilVal *big.Int
	require.Panics(t, func() {
		_, _ = p.Set(thor.KeyMaxBlockProposers, nilVal).Clause()
	})
}

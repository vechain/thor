// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thorclient/bind"
)

func TestEnergy(t *testing.T) {
	testNode, client := newTestNode(t, false)
	defer testNode.Stop()

	energy, err := NewEnergy(client)
	require.NoError(t, err)

	t.Run("Name", func(t *testing.T) {
		name, err := energy.Name()
		require.NoError(t, err)
		require.Equal(t, "VeThor", name)
	})

	t.Run("Symbol", func(t *testing.T) {
		symbol, err := energy.Symbol()
		require.NoError(t, err)
		require.Equal(t, "VTHO", symbol)
	})

	t.Run("Decimals", func(t *testing.T) {
		decimals, err := energy.Decimals()
		require.NoError(t, err)
		require.Equal(t, uint8(18), decimals)
	})

	t.Run("TotalSupply", func(t *testing.T) {
		totalSupply, err := energy.TotalSupply()
		require.NoError(t, err)
		require.Equal(t, 1, totalSupply.Sign())
	})

	t.Run("TotalBurned", func(t *testing.T) {
		totalBurned, err := energy.TotalBurned()
		require.NoError(t, err)
		require.NotNil(t, totalBurned)
	})

	t.Run("BalanceOf", func(t *testing.T) {
		balance, err := energy.BalanceOf(genesis.DevAccounts()[0].Address)
		require.NoError(t, err)
		require.Equal(t, 1, balance.Sign())
	})

	t.Run("Approve-Approval-TransferFrom", func(t *testing.T) {
		acc1 := bind.NewSigner(genesis.DevAccounts()[1].PrivateKey)
		acc2 := bind.NewSigner(genesis.DevAccounts()[2].PrivateKey)
		acc3 := bind.NewSigner(genesis.DevAccounts()[3].PrivateKey)

		allowanceAmount := big.NewInt(1000)

		receipt, _, err := energy.Approve(acc2.Address(), allowanceAmount).Send().WithSigner(acc1).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
		require.NoError(t, err)
		require.False(t, receipt.Reverted, "Transaction should not be reverted")

		approvals, err := energy.FilterApproval(newRange(receipt), nil, logdb.ASC)
		require.NoError(t, err)
		require.Len(t, approvals, 1, "There should be one approval event")

		allowance, err := energy.Allowance(acc1.Address(), acc2.Address())
		require.NoError(t, err)
		require.Equal(t, allowanceAmount, allowance, "Allowance should match the approved amount")

		transferAmount := big.NewInt(500)
		receipt, _, err = energy.TransferFrom(acc1.Address(), acc3.Address(), transferAmount).
			Send().
			WithSigner(acc2).
			WithOptions(txOpts()).
			SubmitAndConfirm(txContext(t))
		require.NoError(t, err)
		require.False(t, receipt.Reverted, "TransferFrom should not be reverted")
	})

	t.Run("Transfer", func(t *testing.T) {
		acc1 := bind.NewSigner(genesis.DevAccounts()[1].PrivateKey)
		random, err := crypto.GenerateKey()
		require.NoError(t, err)
		acc2 := bind.NewSigner(random)

		transferAmount := big.NewInt(999)

		receipt, _, err := energy.Transfer(acc2.Address(), transferAmount).Send().WithSigner(acc1).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
		require.NoError(t, err)
		require.False(t, receipt.Reverted, "Transfer should not be reverted")

		balance, err := energy.BalanceOf(acc2.Address())
		require.NoError(t, err)
		require.Equal(t, transferAmount, balance, "Balance should match the transferred amount")

		transfers, err := energy.FilterTransfer(newRange(receipt), nil, logdb.ASC)
		require.NoError(t, err)

		found := false
		for _, transfer := range transfers {
			if transfer.To == acc2.Address() && transfer.From == acc1.Address() && transfer.Value.Cmp(transferAmount) == 0 {
				found = true
				break
			}
		}
		require.True(t, found, "Transfer event should be found in the logs")
	})
}

func TestEnergy_Revision(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	energy, err := NewEnergy(client)
	require.NoError(t, err)

	require.NoError(t, node.Chain().MintBlock(genesis.DevAccounts()[0]))
	require.NoError(t, node.Chain().MintBlock(genesis.DevAccounts()[0]))

	supplyBlock1, err := energy.Revision("2").TotalSupply()
	require.NoError(t, err)
	supplyBlock2, err := energy.Revision("3").TotalSupply()
	require.NoError(t, err)

	require.Greater(t, supplyBlock2.Cmp(supplyBlock1), 0, "Total supply should increase with each block")
}

// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	contracts "github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logsdb"
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

		approvals, err := energy.FilterApproval(newRange(receipt), nil, logsdb.ASC)
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

		transfers, err := energy.FilterTransfer(newRange(receipt), nil, logsdb.ASC)
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

	hayabusa := int(node.Chain().GetForkConfig().HAYABUSA)
	supplyAtFork, err := energy.Revision(strconv.Itoa(hayabusa)).TotalSupply()
	require.NoError(t, err)
	supplyAfterFork, err := energy.Revision(strconv.Itoa(hayabusa + 1)).TotalSupply()
	require.NoError(t, err)

	require.Equal(t, 0, supplyAfterFork.Cmp(supplyAtFork), "Total supply should stop increasing after the hardfork block")
}

func TestEnergy_RawContract(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	energy, err := NewEnergy(client)
	require.NoError(t, err)

	raw := energy.Raw()
	require.NotNil(t, raw)
	require.Equal(t, contracts.Energy.Address, *raw.Address())
	_, ok := raw.ABI().Methods["name"]
	require.True(t, ok, "expected method 'name' in ABI")
}

func TestEnergy_Move(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	energy, err := NewEnergy(client)
	require.NoError(t, err)

	acc1 := bind.NewSigner(genesis.DevAccounts()[1].PrivateKey)
	acc2 := bind.NewSigner(genesis.DevAccounts()[2].PrivateKey)
	acc3 := bind.NewSigner(genesis.DevAccounts()[3].PrivateKey)

	// Set acc2 as master of acc1 so that acc2 can move acc1's balance
	prototype, err := NewPrototype(client)
	require.NoError(t, err)
	_, _, err = prototype.SetMaster(acc1.Address(), acc2.Address()).
		Send().
		WithSigner(acc1).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t))
	require.NoError(t, err)

	transferAmount := big.NewInt(777)
	receipt, _, err := energy.Move(acc1.Address(), acc3.Address(), transferAmount).
		Send().
		WithSigner(acc2).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	transfers, err := energy.FilterTransfer(newRange(receipt), nil, logsdb.ASC)
	require.NoError(t, err)
	found := false
	for _, tr := range transfers {
		if tr.From == acc1.Address() && tr.To == acc3.Address() && tr.Value.Cmp(transferAmount) == 0 {
			found = true
			break
		}
	}
	require.True(t, found, "Move should emit a matching Transfer event")
}

func TestEnergy_FilterEvents_EventNotFound(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	badContract, err := bind.NewContract(client, contracts.Authority.RawABI(), &contracts.Energy.Address)
	require.NoError(t, err)
	bad := &Energy{contract: badContract}

	testCases := []struct {
		name string
		call func() error
	}{
		{
			name: "Transfer",
			call: func() error {
				_, err := bad.FilterTransfer(nil, nil, logsdb.ASC)
				return err
			},
		},
		{
			name: "Approval",
			call: func() error {
				_, err := bad.FilterApproval(nil, nil, logsdb.ASC)
				return err
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			require.Error(t, err)
		})
	}
}

func TestEnergy_Methods_MethodNotFound(t *testing.T) {
	node, client := newTestNode(t, false)
	defer node.Stop()

	// Use wrong ABI (Authority) so method lookups like name/symbol/... are missing
	badContract, err := bind.NewContract(client, contracts.Authority.RawABI(), &contracts.Energy.Address)
	require.NoError(t, err)
	bad := &Energy{contract: badContract}

	owner := genesis.DevAccounts()[0].Address
	spender := genesis.DevAccounts()[1].Address

	tests := []struct {
		name string
		call func() error
	}{
		{"Name", func() error { _, err := bad.Name(); return err }},
		{"Symbol", func() error { _, err := bad.Symbol(); return err }},
		{"Decimals", func() error { _, err := bad.Decimals(); return err }},
		{"TotalSupply", func() error { _, err := bad.TotalSupply(); return err }},
		{"TotalBurned", func() error { _, err := bad.TotalBurned(); return err }},
		{"BalanceOf", func() error { _, err := bad.BalanceOf(owner); return err }},
		{"Allowance", func() error { _, err := bad.Allowance(owner, spender); return err }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.call())
		})
	}
}

// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/bind"
	"github.com/vechain/thor/v2/tx"
)

func TestPrototype(t *testing.T) {
	_, client := newChain(t, false)
	chainTag, err := client.ChainTag()
	require.NoError(t, err)

	prototype, err := NewPrototype(client)
	require.NoError(t, err)
	accKey := genesis.DevAccounts()[0].PrivateKey
	acc := bind.NewSigner(accKey)
	acc2 := bind.NewSigner(genesis.DevAccounts()[1].PrivateKey)

	contractBytecode := "0x6080604052348015600f57600080fd5b5060c88061001e6000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c8063b8d1c87214602d575b600080fd5b605660048036036020811015604157600080fd5b81019080803590602001909291905050506058565b005b7fa66e3d99cea58d39cb278611964329fa8d4b08252d747eced50565286fb225c0816040518082815260200191505060405180910390a15056fea2646970667358221220be91f5a1548580d479fc71c4ee668fdb51566550b04fa3632f1d4c453053d3e264736f6c63430006020033"
	bytecode, err := hexutil.Decode(contractBytecode)
	require.NoError(t, err)
	contractClause := tx.NewClause(nil).WithData(bytecode)

	trx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(chainTag).
		Clause(contractClause).
		Expiration(10000).
		Gas(10_000_000).
		Build()
	trx = tx.MustSign(trx, accKey)

	res, err := client.SendTransaction(trx)
	require.NoError(t, err)

	var receipt *transactions.Receipt
	require.NoError(t,
		test.Retry(func() error {
			if receipt, err = client.TransactionReceipt(res.ID); err != nil {
				return err
			}
			return nil
		}, time.Second, 10*time.Second))
	contractAddr := receipt.Outputs[0].Events[0].Address

	// Master
	master, err := prototype.Master(contractAddr)
	require.NoError(t, err)
	require.Equal(t, acc.Address(), master)

	// IsUser
	receipt, _, err = prototype.SetMaster(builtin.Authority.Address, builtin.Authority.Address).
		Send().
		WithSigner(acc).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.True(t, receipt.Reverted) // should revert because acc is not the master

	//Balance
	balance, err := prototype.Balance(acc.Address(), big.NewInt(1))
	require.NoError(t, err)
	require.Equal(t, 1, balance.Sign())

	// Energy
	energy, err := prototype.Energy(acc.Address(), big.NewInt(1))
	require.NoError(t, err)
	require.Equal(t, 1, energy.Sign())

	// HasCode
	hasCode, err := prototype.HasCode(builtin.Authority.Address)
	require.NoError(t, err)
	require.True(t, hasCode)

	// StorageFor
	storage, err := prototype.StorageFor(builtin.Authority.Address, thor.Bytes32{})
	require.NoError(t, err)
	require.Equal(t, thor.Bytes32{}, storage)

	// SetCreditPlan
	receipt, _, err = prototype.SetCreditPlan(contractAddr, big.NewInt(100), big.NewInt(200)).
		Send().
		WithSigner(acc).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// CreditPlan
	_, _, err = prototype.CreditPlan(builtin.Authority.Address)
	require.NoError(t, err)

	// AddUser
	receipt, _, err = prototype.AddUser(contractAddr, acc2.Address()).
		Send().
		WithSigner(acc).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// IsUser
	isUser, err := prototype.IsUser(contractAddr, acc2.Address())
	require.NoError(t, err)
	require.True(t, isUser)

	// RemoveUser
	receipt, _, err = prototype.RemoveUser(contractAddr, acc2.Address()).
		Send().
		WithSigner(acc).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// IsUser after RemoveUser
	isUser, err = prototype.IsUser(contractAddr, acc2.Address())
	require.NoError(t, err)
	require.False(t, isUser)

	// SetMaster
	receipt, _, err = prototype.SetMaster(contractAddr, acc2.Address()).
		Send().
		WithSigner(acc).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// Master after SetMaster
	master, err = prototype.Master(contractAddr)
	require.NoError(t, err)
	require.Equal(t, acc2.Address(), master)

	// IsSponsor
	isSponsor, err := prototype.IsSponsor(acc.Address(), contractAddr)
	require.NoError(t, err)
	require.False(t, isSponsor)

	// Sponsor
	receipt, _, err = prototype.Sponsor(contractAddr).Send().WithSigner(acc).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// SelectSponsor
	receipt, _, err = prototype.SelectSponsor(contractAddr, acc.Address()).
		Send().
		WithSigner(acc2).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// CurrentSponsor
	currentSponsor, err := prototype.CurrentSponsor(contractAddr)
	require.NoError(t, err)
	require.Equal(t, acc.Address(), currentSponsor)

	// Unsponsor
	receipt, _, err = prototype.Unsponsor(contractAddr).
		Send().
		WithSigner(acc).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)
}

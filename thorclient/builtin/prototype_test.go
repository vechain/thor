// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/bind"
	"github.com/vechain/thor/v2/tx"
)

func TestPrototype(t *testing.T) {
	_, client := newChain(t, false)
	gene, err := client.GetBlock("0")
	require.NoError(t, err)
	chainTag := gene.ID[31]

	prototype, err := NewPrototype(client)
	require.NoError(t, err)
	accKey := genesis.DevAccounts()[0].PrivateKey
	acc := (*bind.PrivateKeySigner)(accKey)
	acc2 := (*bind.PrivateKeySigner)(genesis.DevAccounts()[1].PrivateKey)

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
	rlpTx, err := trx.MarshalBinary()
	require.NoError(t, err)
	res, err := client.SendTransaction(&transactions.RawTx{Raw: hexutil.Encode(rlpTx)}) // tx mines on API call due to mock tx pool
	require.NoError(t, err)
	receipt, err := client.GetTransactionReceipt(res.ID, "")
	require.NoError(t, err)
	contractAddr := receipt.Outputs[0].Events[0].Address

	// Master
	master, err := prototype.Master(contractAddr)
	require.NoError(t, err)
	require.Equal(t, acc.Address(), master)

	// IsUser
	receipt, _, err = prototype.SetMaster(acc, builtin.Authority.Address, builtin.Authority.Address).Receipt(txContext(t), txOpts())
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
	receipt, _, err = prototype.SetCreditPlan(acc, contractAddr, big.NewInt(100), big.NewInt(200)).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// CreditPlan
	_, _, err = prototype.CreditPlan(builtin.Authority.Address)
	require.NoError(t, err)

	// AddUser
	receipt, _, err = prototype.AddUser(acc, contractAddr, acc2.Address()).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// IsUser
	isUser, err := prototype.IsUser(contractAddr, acc2.Address())
	require.NoError(t, err)
	require.True(t, isUser)

	// RemoveUser
	receipt, _, err = prototype.RemoveUser(acc, contractAddr, acc2.Address()).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// IsUser after RemoveUser
	isUser, err = prototype.IsUser(contractAddr, acc2.Address())
	require.NoError(t, err)
	require.False(t, isUser)

	// SetMaster
	receipt, _, err = prototype.SetMaster(acc, contractAddr, acc2.Address()).Receipt(txContext(t), txOpts())
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
	receipt, _, err = prototype.Sponsor(acc, contractAddr).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// SelectSponsor
	receipt, _, err = prototype.SelectSponsor(acc2, contractAddr, acc.Address()).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	// CurrentSponsor
	currentSponsor, err := prototype.CurrentSponsor(contractAddr)
	require.NoError(t, err)
	require.Equal(t, acc.Address(), currentSponsor)

	// Unsponsor
	receipt, _, err = prototype.Unsponsor(acc, contractAddr).Receipt(txContext(t), txOpts())
	require.NoError(t, err)
	require.False(t, receipt.Reverted)
}

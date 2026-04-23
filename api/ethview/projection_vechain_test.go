// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethview_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api/ethview"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func buildLegacySingleClause(t *testing.T, to *thor.Address, value *big.Int, data []byte, gpc uint8, nonce uint64) *tx.Transaction {
	t.Helper()
	clause := tx.NewClause(to).WithValue(value).WithData(data)
	b := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(0x4a).
		Clause(clause).
		GasPriceCoef(gpc).
		Gas(21000).
		BlockRef(tx.BlockRef{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}).
		Expiration(100).
		Nonce(nonce)
	return tx.MustSign(b.Build(), genesis.DevAccounts()[0].PrivateKey)
}

func buildDynamicFeeSingleClause(t *testing.T, to *thor.Address, value *big.Int, data []byte, nonce uint64) *tx.Transaction {
	t.Helper()
	clause := tx.NewClause(to).WithValue(value).WithData(data)
	b := tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(0x4a).
		Clause(clause).
		MaxFeePerGas(big.NewInt(10_000_000_000_000)).
		MaxPriorityFeePerGas(big.NewInt(1_000_000_000)).
		Gas(21000).
		BlockRef(tx.BlockRef{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}).
		Expiration(200).
		Nonce(nonce)
	return tx.MustSign(b.Build(), genesis.DevAccounts()[0].PrivateKey)
}

func buildLegacyMultiClause(t *testing.T) *tx.Transaction {
	t.Helper()
	c1 := tx.NewClause(nil).WithValue(big.NewInt(1))
	c2 := tx.NewClause(nil).WithValue(big.NewInt(2))
	b := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(0x4a).
		Clause(c1).
		Clause(c2).
		GasPriceCoef(0).
		Gas(42000).
		Nonce(1)
	return tx.MustSign(b.Build(), genesis.DevAccounts()[0].PrivateKey)
}

func TestProjectTx_Legacy_SingleClause_Pending(t *testing.T) {
	to := thor.BytesToAddress([]byte("legacy-to"))
	trx := buildLegacySingleClause(t, &to, big.NewInt(999), []byte{0xde, 0xad}, 7, 123)

	// Caller computes bgp × (255 + gpc) / 255; test passes a synthetic value to
	// verify it's surfaced as gasPrice verbatim.
	effective := big.NewInt(1_250_000_000)

	obj, err := ethview.ProjectTx(trx, ethview.TxMeta{
		Origin:            genesis.DevAccounts()[0].Address,
		EffectiveGasPrice: effective,
	})
	require.NoError(t, err)

	// Eth standard fields copied from the single clause.
	require.NotNil(t, obj.To)
	assert.Equal(t, to, *obj.To)
	assert.Equal(t, 0, big.NewInt(999).Cmp((*big.Int)(obj.Value)))
	assert.Equal(t, []byte{0xde, 0xad}, []byte(obj.Input))
	assert.Equal(t, uint64(tx.TypeLegacy), uint64(obj.Type))
	assert.Nil(t, obj.ChainID, "legacy tx has no chainId field")
	assert.Nil(t, obj.MaxFeePerGas, "legacy tx has no fee caps")
	assert.Nil(t, obj.MaxPriorityFeePerGas)
	assert.Nil(t, obj.AccessList, "legacy tx has no access list")

	// Caller-supplied effective gas price passes through.
	assert.Equal(t, 0, effective.Cmp((*big.Int)(obj.GasPrice)))

	// VeChainTx extension fields populated.
	require.NotNil(t, obj.ChainTag)
	assert.Equal(t, uint64(0x4a), uint64(*obj.ChainTag))
	require.NotNil(t, obj.BlockRef)
	assert.Equal(t, 8, len([]byte(*obj.BlockRef)))
	require.NotNil(t, obj.Expiration)
	assert.Equal(t, uint64(100), uint64(*obj.Expiration))
	require.NotNil(t, obj.GasPriceCoef)
	assert.Equal(t, uint64(7), uint64(*obj.GasPriceCoef))
	require.Len(t, obj.Clauses, 1)

	// No delegation -> reserved omitted, delegator nil.
	assert.Nil(t, obj.Reserved)
	assert.Nil(t, obj.Delegator)
}

func TestProjectTx_DynamicFee_SingleClause_Pending(t *testing.T) {
	to := thor.BytesToAddress([]byte("df-to"))
	trx := buildDynamicFeeSingleClause(t, &to, big.NewInt(500), []byte{0xbe, 0xef}, 9)

	obj, err := ethview.ProjectTx(trx, ethview.TxMeta{Origin: genesis.DevAccounts()[0].Address})
	require.NoError(t, err)

	assert.Equal(t, uint64(tx.TypeDynamicFee), uint64(obj.Type))
	require.NotNil(t, obj.MaxFeePerGas)
	require.NotNil(t, obj.MaxPriorityFeePerGas)
	// Pending 0x51 falls back to maxFeePerGas.
	assert.Equal(t, 0, trx.MaxFeePerGas().Cmp((*big.Int)(obj.GasPrice)))
	// Extension fields present.
	require.NotNil(t, obj.ChainTag)
	require.NotNil(t, obj.BlockRef)
	require.NotNil(t, obj.Expiration)
	assert.Nil(t, obj.GasPriceCoef, "0x51 must not emit gasPriceCoef")
	require.Len(t, obj.Clauses, 1)
}

func TestProjectTx_DynamicFee_SingleClause_Mined(t *testing.T) {
	to := thor.BytesToAddress([]byte("df-mined"))
	trx := buildDynamicFeeSingleClause(t, &to, big.NewInt(0), nil, 1)

	blockID := thor.BytesToBytes32([]byte("b"))
	blockNum := uint32(42)
	effective := big.NewInt(3_000_000_000)

	obj, err := ethview.ProjectTx(trx, ethview.TxMeta{
		BlockID:           &blockID,
		BlockNumber:       &blockNum,
		Index:             2,
		Origin:            genesis.DevAccounts()[0].Address,
		EffectiveGasPrice: effective,
	})
	require.NoError(t, err)

	assert.Equal(t, 0, effective.Cmp((*big.Int)(obj.GasPrice)), "mined 0x51 gasPrice must equal effective")
	require.NotNil(t, obj.BlockHash)
	assert.Equal(t, blockID, *obj.BlockHash)
	require.NotNil(t, obj.BlockNumber)
	assert.Equal(t, uint64(42), uint64(*obj.BlockNumber))
	require.NotNil(t, obj.TransactionIndex)
	assert.Equal(t, uint64(2), uint64(*obj.TransactionIndex))
}

func TestProjectTx_Legacy_MultiClause_NotRepresentable(t *testing.T) {
	trx := buildLegacyMultiClause(t)
	obj, err := ethview.ProjectTx(trx, ethview.TxMeta{Origin: genesis.DevAccounts()[0].Address})
	assert.Nil(t, obj)
	assert.ErrorIs(t, err, ethview.ErrNotRepresentable)
}

func TestProjectTx_Legacy_Delegated_ExposesDelegatorAndReserved(t *testing.T) {
	// Build a delegated 0x00 tx: Features(DelegationFeature=1), sign with both
	// origin and delegator.
	to := thor.BytesToAddress([]byte("d"))
	clause := tx.NewClause(&to).WithValue(big.NewInt(1))
	b := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(0x4a).
		Clause(clause).
		GasPriceCoef(0).
		Gas(21000).
		Nonce(1).
		Features(tx.DelegationFeature)
	raw := b.Build()

	origin := genesis.DevAccounts()[0]
	delegator := genesis.DevAccounts()[1]
	signed, err := tx.SignDelegated(raw, origin.PrivateKey, delegator.PrivateKey)
	require.NoError(t, err)

	obj, err := ethview.ProjectTx(signed, ethview.TxMeta{
		Origin:            origin.Address,
		Delegator:         &delegator.Address,
		EffectiveGasPrice: big.NewInt(1),
	})
	require.NoError(t, err)

	require.NotNil(t, obj.Delegator)
	assert.Equal(t, delegator.Address, *obj.Delegator)
	require.NotNil(t, obj.Reserved)
	assert.Equal(t, uint64(tx.DelegationFeature), uint64(obj.Reserved.Features))
}

func TestProjectTx_Legacy_DependsOn_Surfaced(t *testing.T) {
	to := thor.BytesToAddress([]byte("d2"))
	dep := thor.BytesToBytes32([]byte("dep"))
	clause := tx.NewClause(&to).WithValue(big.NewInt(1))
	b := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(0x4a).
		Clause(clause).
		Gas(21000).
		GasPriceCoef(0).
		Nonce(1).
		DependsOn(&dep)
	trx := tx.MustSign(b.Build(), genesis.DevAccounts()[0].PrivateKey)

	obj, err := ethview.ProjectTx(trx, ethview.TxMeta{
		Origin:            genesis.DevAccounts()[0].Address,
		EffectiveGasPrice: big.NewInt(1),
	})
	require.NoError(t, err)

	require.NotNil(t, obj.DependsOn)
	assert.Equal(t, dep, *obj.DependsOn)
}

func TestProjectTx_JSON_ExtensionFields_OnLegacy(t *testing.T) {
	to := thor.BytesToAddress([]byte("j"))
	trx := buildLegacySingleClause(t, &to, big.NewInt(1), nil, 3, 1)

	obj, err := ethview.ProjectTx(trx, ethview.TxMeta{
		Origin:            genesis.DevAccounts()[0].Address,
		EffectiveGasPrice: big.NewInt(2),
	})
	require.NoError(t, err)

	raw, err := json.Marshal(obj)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	for _, f := range []string{"chainTag", "blockRef", "expiration", "clauses", "gasPriceCoef"} {
		assert.Contains(t, got, f, "legacy projection missing %q", f)
	}
	for _, f := range []string{"chainId", "maxFeePerGas", "maxPriorityFeePerGas", "accessList"} {
		_, present := got[f]
		assert.False(t, present, "legacy projection must not emit %q", f)
	}
}

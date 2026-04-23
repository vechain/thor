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

// buildSigned0x02 constructs and signs an 0x02 tx with an optional `to`. If
// nil, the tx is a contract creation (to=nil in the eth-shape projection).
func buildSigned0x02(t *testing.T, chainID *big.Int, nonce uint64, to *thor.Address, value *big.Int, data []byte) *tx.Transaction {
	t.Helper()
	b := tx.NewBuilder(tx.TypeEthDynamicFee).
		ChainID(chainID).
		EthTo(to).
		EthValue(value).
		MaxFeePerGas(big.NewInt(10_000_000_000_000)).
		MaxPriorityFeePerGas(big.NewInt(1_000_000_000)).
		Gas(21000).
		Nonce(nonce)
	if len(data) > 0 {
		b = b.EthData(data)
	}
	return tx.MustSign(b.Build(), genesis.DevAccounts()[0].PrivateKey)
}

func TestProjectTx_0x02_Pending(t *testing.T) {
	chainID := big.NewInt(6986)
	to := thor.BytesToAddress([]byte("recipient"))
	val := big.NewInt(42)
	trx := buildSigned0x02(t, chainID, 7, &to, val, []byte{0xab, 0xcd})

	meta := ethview.TxMeta{
		Origin: genesis.DevAccounts()[0].Address,
	}

	obj, err := ethview.ProjectTx(trx, meta)
	require.NoError(t, err)
	require.NotNil(t, obj)

	assert.Equal(t, trx.CanonicalTxID(), obj.Hash)
	assert.Equal(t, uint64(tx.TypeEthDynamicFee), uint64(obj.Type))
	require.NotNil(t, obj.ChainID)
	assert.Equal(t, 0, chainID.Cmp((*big.Int)(obj.ChainID)))
	assert.Equal(t, uint64(7), uint64(obj.Nonce))
	assert.Equal(t, genesis.DevAccounts()[0].Address, obj.From)
	require.NotNil(t, obj.To)
	assert.Equal(t, to, *obj.To)
	assert.Equal(t, 0, val.Cmp((*big.Int)(obj.Value)))
	assert.Equal(t, uint64(21000), uint64(obj.Gas))
	// Pending -> gasPrice = maxFeePerGas.
	assert.Equal(t, 0, (*big.Int)(obj.GasPrice).Cmp(trx.MaxFeePerGas()), "pending gasPrice must equal maxFeePerGas")
	assert.Equal(t, 0, trx.MaxFeePerGas().Cmp((*big.Int)(obj.MaxFeePerGas)))
	assert.Equal(t, 0, trx.MaxPriorityFeePerGas().Cmp((*big.Int)(obj.MaxPriorityFeePerGas)))
	require.NotNil(t, obj.AccessList)
	assert.Equal(t, 0, len(*obj.AccessList))
	assert.Equal(t, []byte{0xab, 0xcd}, []byte(obj.Input))
	// Pending -> no block location.
	assert.Nil(t, obj.BlockHash)
	assert.Nil(t, obj.BlockNumber)
	assert.Nil(t, obj.TransactionIndex)
	// Signature triplet decoded.
	require.NotNil(t, obj.R)
	require.NotNil(t, obj.S)
	require.NotNil(t, obj.V)
	assert.Equal(t, 1, (*big.Int)(obj.R).Sign(), "R must be positive")
	assert.Equal(t, 1, (*big.Int)(obj.S).Sign(), "S must be positive")
	// No VeChainTx extension fields on 0x02.
	assert.Nil(t, obj.ChainTag)
	assert.Nil(t, obj.BlockRef)
	assert.Nil(t, obj.Expiration)
	assert.Nil(t, obj.Clauses)
	assert.Nil(t, obj.DependsOn)
	assert.Nil(t, obj.Reserved)
	assert.Nil(t, obj.Delegator)
	assert.Nil(t, obj.GasPriceCoef)
}

func TestProjectTx_0x02_Mined(t *testing.T) {
	chainID := big.NewInt(6986)
	to := thor.BytesToAddress([]byte("to-mined"))
	trx := buildSigned0x02(t, chainID, 1, &to, big.NewInt(0), nil)

	blockID := thor.BytesToBytes32([]byte("block-id"))
	blockNum := uint32(12345)
	effective := big.NewInt(2_500_000_000)

	meta := ethview.TxMeta{
		BlockID:           &blockID,
		BlockNumber:       &blockNum,
		Index:             3,
		Origin:            genesis.DevAccounts()[0].Address,
		EffectiveGasPrice: effective,
	}

	obj, err := ethview.ProjectTx(trx, meta)
	require.NoError(t, err)

	require.NotNil(t, obj.BlockHash)
	assert.Equal(t, blockID, *obj.BlockHash)
	require.NotNil(t, obj.BlockNumber)
	assert.Equal(t, uint64(blockNum), uint64(*obj.BlockNumber))
	require.NotNil(t, obj.TransactionIndex)
	assert.Equal(t, uint64(3), uint64(*obj.TransactionIndex))
	assert.Equal(t, 0, effective.Cmp((*big.Int)(obj.GasPrice)), "mined gasPrice must equal effectiveGasPrice")
}

func TestProjectTx_0x02_ContractCreation(t *testing.T) {
	chainID := big.NewInt(6986)
	trx := buildSigned0x02(t, chainID, 0, nil, big.NewInt(0), []byte{0x60, 0x80, 0x60, 0x40})

	obj, err := ethview.ProjectTx(trx, ethview.TxMeta{Origin: genesis.DevAccounts()[0].Address})
	require.NoError(t, err)
	assert.Nil(t, obj.To, "CREATE tx must project to=nil")
	assert.Equal(t, []byte{0x60, 0x80, 0x60, 0x40}, []byte(obj.Input))
}

func TestProjectTx_0x02_JSONShape(t *testing.T) {
	chainID := big.NewInt(6986)
	to := thor.BytesToAddress([]byte("rcpt"))
	trx := buildSigned0x02(t, chainID, 5, &to, big.NewInt(100), nil)

	obj, err := ethview.ProjectTx(trx, ethview.TxMeta{Origin: genesis.DevAccounts()[0].Address})
	require.NoError(t, err)

	raw, err := json.Marshal(obj)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	// Standard eth fields always present.
	for _, f := range []string{"hash", "type", "chainId", "nonce", "from", "to", "value", "gas", "gasPrice", "maxFeePerGas", "maxPriorityFeePerGas", "accessList", "input", "v", "r", "s"} {
		assert.Contains(t, got, f, "missing json field %q", f)
	}
	// Pending-tx block fields emit as JSON null.
	for _, f := range []string{"blockHash", "blockNumber", "transactionIndex"} {
		v, ok := got[f]
		assert.True(t, ok, "missing json field %q", f)
		assert.Nil(t, v, "pending %q must be json null", f)
	}
	// Extension fields omitted on 0x02.
	for _, f := range []string{"chainTag", "blockRef", "expiration", "clauses", "dependsOn", "reserved", "delegator", "gasPriceCoef"} {
		_, present := got[f]
		assert.False(t, present, "unexpected VeChainTx extension field %q on 0x02 projection", f)
	}
	// Type is hex.
	assert.Equal(t, "0x2", got["type"])
}

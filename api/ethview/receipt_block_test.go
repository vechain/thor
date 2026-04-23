// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package ethview_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api/ethview"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func makeReceipt(reverted bool, outputs []*tx.Output) *tx.Receipt {
	return &tx.Receipt{
		GasUsed:  42000,
		GasPayer: genesis.DevAccounts()[0].Address,
		Paid:     big.NewInt(1_000_000),
		Reward:   big.NewInt(500_000),
		Reverted: reverted,
		Outputs:  outputs,
	}
}

func TestProjectReceipt_0x02_HappyPath(t *testing.T) {
	chainID := big.NewInt(6986)
	to := thor.BytesToAddress([]byte("r-to"))
	trx := buildSigned0x02(t, chainID, 1, &to, big.NewInt(0), nil)

	receipt := makeReceipt(false, []*tx.Output{{
		Events:    tx.Events{{Address: to, Topics: []thor.Bytes32{thor.BytesToBytes32([]byte("t"))}, Data: []byte{0x01}}},
		Transfers: tx.Transfers{},
	}})

	blockID := thor.BytesToBytes32([]byte("block"))
	blockNum := uint32(10)
	effective := big.NewInt(2_000_000_000)

	obj, err := ethview.ProjectReceipt(trx, receipt, ethview.TxMeta{
		BlockID:           &blockID,
		BlockNumber:       &blockNum,
		Index:             0,
		Origin:            genesis.DevAccounts()[0].Address,
		EffectiveGasPrice: effective,
	}, 42000, nil)
	require.NoError(t, err)

	assert.Equal(t, trx.CanonicalTxID(), obj.TransactionHash)
	assert.Equal(t, blockID, obj.BlockHash)
	assert.Equal(t, uint64(blockNum), uint64(obj.BlockNumber))
	assert.Equal(t, uint64(0), uint64(obj.TransactionIndex))
	assert.Equal(t, uint64(1), uint64(obj.Status), "non-reverted receipt has status=1")
	assert.Equal(t, uint64(tx.TypeEthDynamicFee), uint64(obj.Type))
	assert.Equal(t, 0, effective.Cmp((*big.Int)(obj.EffectiveGasPrice)))
	require.Len(t, obj.Logs, 1)
	assert.Equal(t, uint64(0), uint64(obj.Logs[0].LogIndex))
	assert.Equal(t, uint64(0), uint64(obj.Logs[0].TransactionIndex))
	assert.Equal(t, 256, len([]byte(obj.LogsBloom)), "logsBloom is 256 zero bytes")
	assert.Nil(t, obj.GasPayer, "0x02 receipt has no VeChainTx extensions")
	assert.Nil(t, obj.Paid)
	assert.Nil(t, obj.Reward)
	assert.Nil(t, obj.Reverted)
	assert.Nil(t, obj.Transfers)
	assert.Nil(t, obj.Outputs)
}

func TestProjectReceipt_Legacy_SingleClauseExtensions(t *testing.T) {
	to := thor.BytesToAddress([]byte("r-leg"))
	trx := buildLegacySingleClause(t, &to, big.NewInt(0), nil, 0, 1)

	receipt := makeReceipt(false, []*tx.Output{{
		Events: tx.Events{{Address: to, Topics: []thor.Bytes32{thor.BytesToBytes32([]byte("e"))}, Data: nil}},
		Transfers: tx.Transfers{{
			Sender:    genesis.DevAccounts()[0].Address,
			Recipient: to,
			Amount:    big.NewInt(1_000),
		}},
	}})

	blockID := thor.BytesToBytes32([]byte("b-leg"))
	blockNum := uint32(50)

	obj, err := ethview.ProjectReceipt(trx, receipt, ethview.TxMeta{
		BlockID:           &blockID,
		BlockNumber:       &blockNum,
		Index:             1,
		Origin:            genesis.DevAccounts()[0].Address,
		EffectiveGasPrice: big.NewInt(1),
	}, 42000, nil)
	require.NoError(t, err)

	require.NotNil(t, obj.GasPayer)
	assert.Equal(t, genesis.DevAccounts()[0].Address, *obj.GasPayer)
	require.NotNil(t, obj.Paid)
	assert.Equal(t, 0, big.NewInt(1_000_000).Cmp((*big.Int)(obj.Paid)))
	require.NotNil(t, obj.Reverted)
	assert.False(t, *obj.Reverted)
	require.Len(t, obj.Transfers, 1)
	assert.Equal(t, 0, big.NewInt(1_000).Cmp((*big.Int)(obj.Transfers[0].Amount)))
	require.Len(t, obj.Outputs, 1)
	require.Len(t, obj.Outputs[0].Events, 1)
}

func TestProjectReceipt_Reverted_SetsStatusZero(t *testing.T) {
	chainID := big.NewInt(6986)
	to := thor.BytesToAddress([]byte("rev"))
	trx := buildSigned0x02(t, chainID, 1, &to, big.NewInt(0), nil)

	receipt := makeReceipt(true, []*tx.Output{{}})
	blockID := thor.BytesToBytes32([]byte("b"))
	bn := uint32(5)
	obj, err := ethview.ProjectReceipt(trx, receipt, ethview.TxMeta{
		BlockID: &blockID, BlockNumber: &bn, Origin: genesis.DevAccounts()[0].Address,
	}, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), uint64(obj.Status), "reverted status=0")
}

func TestProjectReceipt_MultiClause_NotRepresentable(t *testing.T) {
	trx := buildLegacyMultiClause(t)
	obj, err := ethview.ProjectReceipt(trx, makeReceipt(false, nil), ethview.TxMeta{
		Origin: genesis.DevAccounts()[0].Address,
	}, 0, nil)
	assert.Nil(t, obj)
	assert.ErrorIs(t, err, ethview.ErrNotRepresentable)
}

func TestProjectReceipt_Create_ContractAddressSet(t *testing.T) {
	chainID := big.NewInt(6986)
	trx := buildSigned0x02(t, chainID, 0, nil, big.NewInt(0), []byte{0x60, 0x80})

	receipt := makeReceipt(false, []*tx.Output{{}})
	bn := uint32(1)
	bID := thor.BytesToBytes32([]byte("c"))
	obj, err := ethview.ProjectReceipt(trx, receipt, ethview.TxMeta{
		BlockID: &bID, BlockNumber: &bn, Origin: genesis.DevAccounts()[0].Address,
	}, 0, nil)
	require.NoError(t, err)

	require.NotNil(t, obj.ContractAddress, "CREATE tx must surface contractAddress")
	expected := thor.CreateContractAddress(trx.ID(), 0, 0)
	assert.Equal(t, expected, *obj.ContractAddress)
}

// --- block projection --------------------------------------------------------

func buildSingleTxBlock(t *testing.T, trx *tx.Transaction) *block.Block {
	t.Helper()
	b := new(block.Builder).
		ParentID(thor.BytesToBytes32([]byte("parent"))).
		GasLimit(10_000_000).
		Transaction(trx).
		Build()
	return b
}

func TestProjectBlock_HashesOnly(t *testing.T) {
	chainID := big.NewInt(6986)
	to := thor.BytesToAddress([]byte("x"))
	trx := buildSigned0x02(t, chainID, 1, &to, big.NewInt(0), nil)
	blk := buildSingleTxBlock(t, trx)

	obj, err := ethview.ProjectBlock(blk, false, nil)
	require.NoError(t, err)

	hashes, ok := obj.Transactions.([]thor.Bytes32)
	require.True(t, ok, "fullTx=false transactions must be []Bytes32; got %T", obj.Transactions)
	require.Len(t, hashes, 1)
	assert.Equal(t, trx.CanonicalTxID(), hashes[0])

	assert.Equal(t, 256, len([]byte(obj.LogsBloom)))
	assert.Equal(t, 32, len([]byte(obj.Sha3Uncles[:])))
}

func TestProjectBlock_FullTx_Representable(t *testing.T) {
	chainID := big.NewInt(6986)
	trx := buildSigned0x02(t, chainID, 0, nil, big.NewInt(0), nil)
	blk := buildSingleTxBlock(t, trx)

	obj, err := ethview.ProjectBlock(blk, true, func(idx int) ethview.TxMeta {
		blockID := blk.Header().ID()
		bn := blk.Header().Number()
		return ethview.TxMeta{
			BlockID:     &blockID,
			BlockNumber: &bn,
			Index:       uint32(idx),
			Origin:      genesis.DevAccounts()[0].Address,
		}
	})
	require.NoError(t, err)

	full, ok := obj.Transactions.([]*ethview.TransactionObject)
	require.True(t, ok, "fullTx=true transactions must be []*TransactionObject; got %T", obj.Transactions)
	require.Len(t, full, 1)
	assert.Equal(t, trx.CanonicalTxID(), full[0].Hash)
}

func TestProjectBlock_FullTx_MultiClauseFails(t *testing.T) {
	multi := buildLegacyMultiClause(t)
	blk := buildSingleTxBlock(t, multi)

	obj, err := ethview.ProjectBlock(blk, true, func(idx int) ethview.TxMeta {
		return ethview.TxMeta{Origin: genesis.DevAccounts()[0].Address}
	})
	assert.Nil(t, obj)
	assert.ErrorIs(t, err, ethview.ErrBlockContainsNonRepresentable)
}

func TestProjectBlock_FullTxFalse_IgnoresMultiClause(t *testing.T) {
	multi := buildLegacyMultiClause(t)
	blk := buildSingleTxBlock(t, multi)

	// fullTx=false returns hashes regardless of representability.
	obj, err := ethview.ProjectBlock(blk, false, nil)
	require.NoError(t, err)
	hashes := obj.Transactions.([]thor.Bytes32)
	require.Len(t, hashes, 1)
	assert.Equal(t, multi.CanonicalTxID(), hashes[0])
}

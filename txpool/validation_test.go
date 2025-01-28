// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestValidateTransaction(t *testing.T) {
	db := muxdb.NewMem()
	repo := newChainRepo(db)

	tests := []struct {
		name        string
		getTx       func() *tx.Transaction
		head        *chain.BlockSummary
		forkConfig  *thor.ForkConfig
		expectedErr error
	}{
		{
			name:        "invalid legacy tx chain tag",
			getTx:       func() *tx.Transaction { t, _ := tx.NewTxBuilder(tx.LegacyTxType).ChainTag(0xff).Build(); return t },
			head:        &chain.BlockSummary{},
			forkConfig:  &thor.NoFork,
			expectedErr: badTxError{"chain tag mismatch"},
		},
		{
			name:        "invalid dyn fee tx chain tag",
			getTx:       func() *tx.Transaction { t, _ := tx.NewTxBuilder(tx.DynamicFeeTxType).ChainTag(0xff).Build(); return t },
			head:        &chain.BlockSummary{},
			forkConfig:  &thor.NoFork,
			expectedErr: badTxError{"chain tag mismatch"},
		},
		{
			name: "legacy tx size too large",
			getTx: func() *tx.Transaction {
				b := tx.NewTxBuilder(tx.LegacyTxType).ChainTag(repo.ChainTag())
				// Including a lot of clauses to increase the size above the max allowed
				for i := 0; i < 50_000; i++ {
					b.Clause(&tx.Clause{})
				}
				t, _ := b.Build()
				return t
			},
			head:        &chain.BlockSummary{},
			forkConfig:  &thor.NoFork,
			expectedErr: txRejectedError{"size too large"},
		},
		{
			name: "dyn fee tx size too large",
			getTx: func() *tx.Transaction {
				b := tx.NewTxBuilder(tx.DynamicFeeTxType).ChainTag(repo.ChainTag())
				// Including a lot of clauses to increase the size above the max allowed
				for i := 0; i < 50_000; i++ {
					b.Clause(&tx.Clause{})
				}
				t, _ := b.Build()
				return t
			},
			head:        &chain.BlockSummary{},
			forkConfig:  &thor.NoFork,
			expectedErr: txRejectedError{"size too large"},
		},
		{
			name: "supported legacy transaction type before Galactica fork",
			getTx: func() *tx.Transaction {
				t, _ := tx.NewTxBuilder(tx.LegacyTxType).ChainTag(repo.ChainTag()).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: getHeader(1)},
			forkConfig:  &thor.ForkConfig{GALACTICA: 10},
			expectedErr: nil,
		},
		{
			name: "supported legacy transaction type after Galactica fork",
			getTx: func() *tx.Transaction {
				t, _ := tx.NewTxBuilder(tx.LegacyTxType).ChainTag(repo.ChainTag()).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: getHeader(100)},
			forkConfig:  &thor.ForkConfig{GALACTICA: 10},
			expectedErr: nil,
		},
		{
			name: "unsupported dyn fee transaction type before Galactica fork",
			getTx: func() *tx.Transaction {
				t, _ := tx.NewTxBuilder(tx.DynamicFeeTxType).ChainTag(repo.ChainTag()).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: getHeader(1)},
			forkConfig:  &thor.ForkConfig{GALACTICA: 10},
			expectedErr: tx.ErrTxTypeNotSupported,
		},
		{
			name: "supported dyn fee transaction type after Galactica fork",
			getTx: func() *tx.Transaction {
				t, _ := tx.NewTxBuilder(tx.DynamicFeeTxType).ChainTag(repo.ChainTag()).MaxFeePerGas(big.NewInt(1000)).MaxPriorityFeePerGas(big.NewInt(10)).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: getHeader(100)},
			forkConfig:  &thor.ForkConfig{GALACTICA: 10},
			expectedErr: nil,
		},
		{
			name: "legacy transaction with unsupported features",
			getTx: func() *tx.Transaction {
				t, _ := tx.NewTxBuilder(tx.LegacyTxType).ChainTag(repo.ChainTag()).Features(tx.Features(4)).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: new(block.Builder).TransactionFeatures(tx.Features(1)).Build().Header()},
			forkConfig:  &thor.NoFork,
			expectedErr: txRejectedError{"unsupported features"},
		},
		{
			name: "dyn fee transaction with unsupported features",
			getTx: func() *tx.Transaction {
				t, _ := tx.NewTxBuilder(tx.DynamicFeeTxType).ChainTag(repo.ChainTag()).MaxFeePerGas(big.NewInt(10_000)).MaxPriorityFeePerGas(big.NewInt(100)).Features(tx.Features(4)).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: new(block.Builder).TransactionFeatures(tx.Features(1)).Build().Header()},
			forkConfig:  &thor.ForkConfig{GALACTICA: 0},
			expectedErr: txRejectedError{"unsupported features"},
		},
		{
			name: "legacy transaction with supported features",
			getTx: func() *tx.Transaction {
				t, _ := tx.NewTxBuilder(tx.LegacyTxType).ChainTag(repo.ChainTag()).Features(tx.DelegationFeature).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: getHeader(1)},
			forkConfig:  &thor.NoFork,
			expectedErr: nil,
		},
		{
			name: "dyn fee transaction with supported features",
			getTx: func() *tx.Transaction {
				t, _ := tx.NewTxBuilder(tx.DynamicFeeTxType).ChainTag(repo.ChainTag()).Features(tx.DelegationFeature).MaxFeePerGas(big.NewInt(1000)).MaxPriorityFeePerGas(big.NewInt(10)).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: getHeader(1)},
			forkConfig:  &thor.ForkConfig{GALACTICA: 0},
			expectedErr: nil,
		},
		{
			name: "max fee per gas less than max priority fee per gas",
			getTx: func() *tx.Transaction {
				t, _ := tx.NewTxBuilder(tx.DynamicFeeTxType).ChainTag(repo.ChainTag()).MaxFeePerGas(big.NewInt(10)).MaxPriorityFeePerGas(big.NewInt(100)).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: getHeader(1)},
			forkConfig:  &thor.ForkConfig{GALACTICA: 0},
			expectedErr: txRejectedError{"max fee per gas (10) must be greater than max priority fee per gas (100)\n"},
		},
		{
			name: "max fee per gas is negative",
			getTx: func() *tx.Transaction {
				t, _ := tx.NewTxBuilder(tx.DynamicFeeTxType).ChainTag(repo.ChainTag()).MaxFeePerGas(big.NewInt(-10)).MaxPriorityFeePerGas(big.NewInt(-100)).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: getHeader(1)},
			forkConfig:  &thor.ForkConfig{GALACTICA: 0},
			expectedErr: txRejectedError{"max fee per gas must be positive"},
		},
		{
			name: "max priority fee per gas is negative",
			getTx: func() *tx.Transaction {
				t, _ := tx.NewTxBuilder(tx.DynamicFeeTxType).ChainTag(repo.ChainTag()).MaxFeePerGas(big.NewInt(10)).MaxPriorityFeePerGas(big.NewInt(-100)).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: getHeader(1)},
			forkConfig:  &thor.ForkConfig{GALACTICA: 0},
			expectedErr: txRejectedError{"max priority fee per gas must be positive"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransaction(tt.getTx(), repo, tt.head, tt.forkConfig)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}

func TestValidateTransactionWithState(t *testing.T) {
	db := muxdb.NewMem()
	repo := newChainRepo(db)
	stater := state.NewStater(db)
	state := stater.NewState(repo.GenesisBlock().Header().StateRoot(), 0, 0, 0)

	tests := []struct {
		name        string
		getTx       func() *tx.Transaction
		head        *chain.BlockSummary
		forkConfig  *thor.ForkConfig
		expectedErr error
	}{
		{
			name: "dyn fee tx with not enough fee to pay for base fee",
			getTx: func() *tx.Transaction {
				maxFee := big.NewInt(thor.InitialBaseFee - 1)
				t, _ := tx.NewTxBuilder(tx.DynamicFeeTxType).ChainTag(repo.ChainTag()).MaxFeePerGas(maxFee).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: getHeader(1)},
			forkConfig:  &thor.ForkConfig{GALACTICA: 0},
			expectedErr: txRejectedError{"max fee per gas too low to cover for base fee"},
		},
		{
			name: "dyn fee tx with max fee equals to base fee + 1",
			getTx: func() *tx.Transaction {
				maxFee := big.NewInt(thor.InitialBaseFee + 1)
				t, _ := tx.NewTxBuilder(tx.DynamicFeeTxType).ChainTag(repo.ChainTag()).MaxFeePerGas(maxFee).Build()
				return t
			},
			head:        &chain.BlockSummary{Header: getHeader(1)},
			forkConfig:  &thor.ForkConfig{GALACTICA: 0},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransactionWithState(tt.getTx(), tt.head, tt.forkConfig, state)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}

func getHeader(number uint32) *block.Header {
	var parentID thor.Bytes32
	binary.BigEndian.PutUint32(parentID[:], number)
	return new(block.Builder).TransactionFeatures(tx.Features(1)).BaseFee(big.NewInt(thor.InitialBaseFee)).ParentID(parentID).Build().Header()
}

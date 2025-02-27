// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/fork"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

func TestValidateBlockBody(t *testing.T) {
	db := muxdb.NewMem()
	gen := genesis.NewDevnet()
	stater := state.NewStater(db)

	parent, _, _, err := gen.Build(stater)
	assert.NoError(t, err)

	repo, err := chain.NewRepository(db, parent)
	assert.NoError(t, err)

	tests := []struct {
		name          string
		getBlock      func() *block.Block
		forkConfig    *thor.ForkConfig
		expectedError error
	}{
		{
			name: "supported legacy tx type before galactica fork",
			getBlock: func() *block.Block {
				tr, err := tx.NewTxBuilder(tx.TypeLegacy).ChainTag(repo.ChainTag()).Expiration(10).Build()
				assert.NoError(t, err)
				tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
				return new(block.Builder).Transaction(tr).Build()
			},
			forkConfig:    &thor.ForkConfig{GALACTICA: 10},
			expectedError: nil,
		},
		{
			name: "unsupported tx type before galactica fork",
			getBlock: func() *block.Block {
				tr, err := tx.NewTxBuilder(tx.TypeDynamicFee).ChainTag(repo.ChainTag()).Expiration(10).Build()
				assert.NoError(t, err)
				tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
				return new(block.Builder).Transaction(tr).Build()
			},
			forkConfig:    &thor.ForkConfig{GALACTICA: 10},
			expectedError: consensusError(tx.ErrTxTypeNotSupported.Error()),
		},
		{
			name: "supported legacy tx type after galactica fork",
			getBlock: func() *block.Block {
				tr, err := tx.NewTxBuilder(tx.TypeLegacy).ChainTag(repo.ChainTag()).Expiration(10).Build()
				assert.NoError(t, err)
				tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
				return new(block.Builder).Transaction(tr).Build()
			},
			forkConfig:    &thor.ForkConfig{GALACTICA: 0},
			expectedError: nil,
		},
		{
			name: "supported tx type after galactica fork",
			getBlock: func() *block.Block {
				tr, err := tx.NewTxBuilder(tx.TypeDynamicFee).ChainTag(repo.ChainTag()).Expiration(10).Build()
				assert.NoError(t, err)
				tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
				return new(block.Builder).Transaction(tr).Build()
			},
			forkConfig:    &thor.ForkConfig{GALACTICA: 0},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(repo, stater, tt.forkConfig)

			err := c.validateBlockBody(tt.getBlock())
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func TestValidateBlock(t *testing.T) {
	db := muxdb.NewMem()
	gen := genesis.NewDevnet()
	stater := state.NewStater(db)

	genesis, _, _, err := gen.Build(stater)
	assert.NoError(t, err)

	state := stater.NewState(trie.Root{Hash: genesis.Header().StateRoot()})
	repo, err := chain.NewRepository(db, genesis)
	assert.NoError(t, err)

	tests := []struct {
		name    string
		testFun func(t *testing.T)
	}{
		{
			name: "valid block",
			testFun: func(t *testing.T) {
				baseFee := big.NewInt(100000)
				tr := tx.NewTxBuilder(tx.TypeDynamicFee).ChainTag(repo.ChainTag()).BlockRef(tx.NewBlockRef(10)).MaxFeePerGas(new(big.Int).Sub(baseFee, common.Big1)).MustBuild()
				blk := new(block.Builder).BaseFee(baseFee).Transaction(tr).Build()

				c := New(repo, stater, thor.ForkConfig{GALACTICA: 0})
				s, r, err := c.verifyBlock(blk, state, 0)
				assert.Nil(t, s)
				assert.Nil(t, r)
				assert.Equal(t, fork.ErrMaxFeePerGasTooLow, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFun)
	}
}

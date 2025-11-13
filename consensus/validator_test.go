// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
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
				tr := tx.NewBuilder(tx.TypeLegacy).ChainTag(repo.ChainTag()).Expiration(10).Build()
				tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
				return new(block.Builder).Transaction(tr).Build()
			},
			forkConfig:    &thor.ForkConfig{GALACTICA: 10},
			expectedError: nil,
		},
		{
			name: "unsupported tx type before galactica fork",
			getBlock: func() *block.Block {
				tr := tx.NewBuilder(tx.TypeDynamicFee).ChainTag(repo.ChainTag()).Expiration(10).Build()
				tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
				return new(block.Builder).Transaction(tr).Build()
			},
			forkConfig:    &thor.ForkConfig{GALACTICA: 10},
			expectedError: consensusError("invalid tx: " + tx.ErrTxTypeNotSupported.Error()),
		},
		{
			name: "supported legacy tx type after galactica fork",
			getBlock: func() *block.Block {
				tr := tx.NewBuilder(tx.TypeLegacy).ChainTag(repo.ChainTag()).Expiration(10).Build()
				tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
				return new(block.Builder).Transaction(tr).Build()
			},
			forkConfig:    &thor.ForkConfig{GALACTICA: 0},
			expectedError: nil,
		},
		{
			name: "supported tx type after galactica fork",
			getBlock: func() *block.Block {
				tr := tx.NewBuilder(tx.TypeDynamicFee).ChainTag(repo.ChainTag()).Expiration(10).Build()
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

	genesisBlock, _, _, err := gen.Build(stater)
	assert.NoError(t, err)

	state := stater.NewState(trie.Root{Hash: genesisBlock.Header().StateRoot()})
	repo, err := chain.NewRepository(db, genesisBlock)
	assert.NoError(t, err)

	tests := []struct {
		name    string
		testFun func(t *testing.T)
	}{
		{
			name: "valid block",
			testFun: func(t *testing.T) {
				baseFee := big.NewInt(100000)
				tr := tx.MustSign(
					tx.NewBuilder(tx.TypeDynamicFee).
						ChainTag(repo.ChainTag()).
						BlockRef(tx.NewBlockRef(10)).
						MaxFeePerGas(new(big.Int).Sub(baseFee, common.Big1)).
						Gas(21000).
						Build(),
					genesis.DevAccounts()[0].PrivateKey,
				)
				blk := new(block.Builder).BaseFee(baseFee).Transaction(tr).Build()

				c := New(repo, stater, &thor.ForkConfig{GALACTICA: 0})
				s, r, err := c.verifyBlock(blk, state, 0, false)
				assert.Nil(t, s)
				assert.Nil(t, r)
				assert.ErrorContains(t, err, "gas price is less than block base fee")
			},
		},
		{
			name: "legacy tx with high base fee",
			testFun: func(t *testing.T) {
				baseFee := big.NewInt(thor.InitialBaseFee * 1000) // Base fee higher than legacy tx base gas price
				tr := tx.NewBuilder(tx.TypeLegacy).ChainTag(repo.ChainTag()).Gas(21000).BlockRef(tx.NewBlockRef(10)).Gas(21000).Build()
				tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
				blk := new(block.Builder).
					BaseFee(baseFee).
					Transaction(tr).
					GasUsed(21000).
					ReceiptsRoot(thor.BytesToBytes32(hexutil.MustDecode("0x18e50e1cc2cededa9037a4d89ef5c0147fa104cf15f6a1e97a5ac0cbd4f58422"))).
					StateRoot(thor.BytesToBytes32(hexutil.MustDecode("0xfd52b74feb856784be141440cc8d68d8a518aaa5e845ceed2ed8322f99c11352"))).
					Build()

				c := New(repo, stater, &thor.ForkConfig{GALACTICA: 0})
				s, r, err := c.verifyBlock(blk, state, 0, false)
				assert.Nil(t, s)
				assert.Nil(t, r)
				assert.ErrorContains(t, err, "gas price is less than block base fee")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFun)
	}
}

func TestValidate_NegativeCases(t *testing.T) {
	db := muxdb.NewMem()
	gen := genesis.NewDevnet()
	stater := state.NewStater(db)

	genesisBlock, _, _, err := gen.Build(stater)
	assert.NoError(t, err)

	state := stater.NewState(trie.Root{Hash: genesisBlock.Header().StateRoot()})
	repo, err := chain.NewRepository(db, genesisBlock)
	assert.NoError(t, err)

	baseFee := big.NewInt(100000)
	tr := tx.MustSign(
		tx.NewBuilder(tx.TypeDynamicFee).
			ChainTag(repo.ChainTag()).
			BlockRef(tx.NewBlockRef(10)).
			MaxFeePerGas(new(big.Int).Sub(baseFee, common.Big1)).
			Gas(21000).
			Build(),
		genesis.DevAccounts()[0].PrivateKey,
	)
	blk := new(block.Builder).
		BaseFee(baseFee).
		Timestamp(genesisBlock.Header().Timestamp() + 10).
		TotalScore(uint64(1)).GasLimit(10000000).
		Transaction(tr).
		Build()
	var sig [146]byte
	rand.Read(sig[:])
	blk = blk.WithSignature(sig[:])

	c := New(repo, stater, &thor.ForkConfig{GALACTICA: 0})

	activeGroupSizeSlot := thor.BytesToBytes32([]byte(("validations-active-group-size")))
	state.SetRawStorage(builtin.Staker.Address, activeGroupSizeSlot, rlp.RawValue{0xFF})
	s, r, err := c.validate(state, blk, genesisBlock.Header(), genesisBlock.Header().Timestamp()+10, uint32(0))
	assert.Nil(t, s)
	assert.Nil(t, r)
	assert.Error(t, err)
}

func TestValidateStakingProposer_LockedVETError(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)

	mockRepo := &chain.Repository{}
	mockForkConfig := &thor.ForkConfig{}

	consensus := New(mockRepo, stater, mockForkConfig)

	parent := &block.Header{}

	st := stater.NewState(trie.Root{})

	stakerAddr := builtin.Staker.Address
	st.SetCode(stakerAddr, builtin.Staker.RuntimeBytecodes())

	paramsAddr := builtin.Params.Address
	st.SetCode(paramsAddr, builtin.Params.RuntimeBytecodes())

	paramKey := thor.BytesToBytes32([]byte("some_param"))
	paramValue := []byte("valid_value")
	st.SetStorage(paramsAddr, paramKey, thor.BytesToBytes32(paramValue))

	staker := builtin.Staker.Native(st)

	builder := new(block.Builder).
		ParentID(thor.Bytes32{}).
		Timestamp(1000).
		GasLimit(1000000).
		GasUsed(0).
		TotalScore(0).
		StateRoot(thor.Bytes32{}).
		ReceiptsRoot(thor.Bytes32{}).
		Beneficiary(thor.Address{})

	blk := builder.Build()
	validSignature := make([]byte, 65)
	copy(validSignature, []byte("valid_signature_65_bytes_long_for_testing"))
	blk = blk.WithSignature(validSignature)
	header := blk.Header()

	_, err := consensus.validateStakingProposer(header, parent, staker)
	assert.ErrorContains(t, err, "pos - block signer invalid")
}

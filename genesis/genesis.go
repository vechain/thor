// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"math/big"

	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Genesis to build genesis block.
type Genesis struct {
	builder *Builder
	id      thor.Bytes32
	name    string
}

// Build build the genesis block.
func (g *Genesis) Build(stater *state.Stater) (blk *block.Block, events tx.Events, transfers tx.Transfers, err error) {
	block, events, transfers, err := g.builder.Build(stater)
	if err != nil {
		return nil, nil, nil, err
	}
	if block.Header().ID() != g.id {
		panic("built genesis ID incorrect")
	}
	return block, events, transfers, nil
}

// ID returns genesis block ID.
func (g *Genesis) ID() thor.Bytes32 {
	return g.id
}

// Name returns network name.
func (g *Genesis) Name() string {
	return g.name
}

func mustEncodeInput(abi *abi.ABI, name string, args ...interface{}) []byte {
	m, found := abi.MethodByName(name)
	if !found {
		panic("method not found")
	}
	data, err := m.EncodeInput(args...)
	if err != nil {
		panic(err)
	}
	return data
}

// NewDevnet create genesis for solo mode.
func NewDevnetCustomTimestamp(timestamp uint64) *Genesis {
	launchTime := timestamp

	executor := DevAccounts()[0].Address
	soloBlockSigner := DevAccounts()[0]

	builder := new(Builder).
		GasLimit(thor.InitialGasLimit).
		Timestamp(launchTime).
		State(func(state *state.State) error {

			// setup builtin contracts
			if err := state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Extension.Address, builtin.Extension.RuntimeBytecodes()); err != nil {
				return err
			}

			tokenSupply := &big.Int{}
			energySupply := &big.Int{}
			for _, a := range DevAccounts() {
				bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
				if err := state.SetBalance(a.Address, bal); err != nil {
					return err
				}
				if err := state.SetEnergy(a.Address, bal, launchTime); err != nil {
					return err
				}
				tokenSupply.Add(tokenSupply, bal)
				energySupply.Add(energySupply, bal)
			}
			return builtin.Energy.Native(state, launchTime).SetInitialSupply(tokenSupply, energySupply)
		}).
		Call(
			tx.NewClause(&builtin.Params.Address).WithData(mustEncodeInput(builtin.Params.ABI, "set", thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))),
			thor.Address{}).
		Call(
			tx.NewClause(&builtin.Params.Address).WithData(mustEncodeInput(builtin.Params.ABI, "set", thor.KeyRewardRatio, thor.InitialRewardRatio)),
			executor).
		Call(
			tx.NewClause(&builtin.Params.Address).WithData(mustEncodeInput(builtin.Params.ABI, "set", thor.KeyBaseGasPrice, thor.InitialBaseGasPrice)),
			executor).
		Call(
			tx.NewClause(&builtin.Params.Address).WithData(mustEncodeInput(builtin.Params.ABI, "set", thor.KeyProposerEndorsement, thor.InitialProposerEndorsement)),
			executor).
		Call(
			tx.NewClause(&builtin.Authority.Address).WithData(mustEncodeInput(builtin.Authority.ABI, "add", soloBlockSigner.Address, soloBlockSigner.Address, thor.BytesToBytes32([]byte("Solo Block Signer")))),
			executor)

	id, err := builder.ComputeID()
	if err != nil {
		panic(err)
	}

	return &Genesis{builder, id, "devnet"}
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"math/big"

	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// New193Net creates 193 testnet genesis.
func New193Net() *Genesis {
	launchTime := uint64(1617091200) //'2021-03-30T08:00:00Z'

	endorsor := thor.MustParseAddress("0x7dd8f15f94f1877e5c3ba175eabcf6743926060e")
	rich := thor.MustParseAddress("0xa477d5b50daf4c673308da10eecfc817eb9f5f21")
	executor := thor.MustParseAddress("0x0d44fec5432ec2395b92d8d96340f16a5d192519")

	initialAuthorityNodes := load193AuthorityNodes()

	builder := new(Builder).
		Timestamp(launchTime).
		GasLimit(thor.InitialGasLimit).
		ForkConfig(thor.NoFork).
		State(func(state *state.State) error {
			// alloc builtin contracts
			if err := state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Energy.Address, builtin.Energy.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Extension.Address, builtin.Extension.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes()); err != nil {
				return err
			}

			tokenSupply := &big.Int{}
			energySupply := &big.Int{}

			tokenSupply.Add(tokenSupply, thor.InitialProposerEndorsement)
			if err := state.SetBalance(endorsor, thor.InitialProposerEndorsement); err != nil {
				return err
			}
			if err := state.SetEnergy(endorsor, &big.Int{}, launchTime); err != nil {
				return err
			}

			// alloc tokens for rich account
			amount := new(big.Int).Mul(big.NewInt(50*1000*1000*1000), big.NewInt(1e18))
			energy := new(big.Int).Mul(big.NewInt(1*1000*1000*1000), big.NewInt(1e18))
			tokenSupply.Add(tokenSupply, amount)
			energySupply.Add(energySupply, energy)
			if err := state.SetBalance(rich, amount); err != nil {
				return err
			}
			if err := state.SetEnergy(rich, energy, launchTime); err != nil {
				return err
			}

			return builtin.Energy.Native(state, launchTime).SetInitialSupply(tokenSupply, energySupply)
		})

	///// initialize builtin contracts

	// initialize params
	data := mustEncodeInput(builtin.Params.ABI, "set", thor.KeyExecutorAddress, new(big.Int).SetBytes(executor.Bytes()))
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), thor.Address{})

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyRewardRatio, thor.InitialRewardRatio)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyBaseGasPrice, thor.InitialBaseGasPrice)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyProposerEndorsement, thor.InitialProposerEndorsement)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	data = mustEncodeInput(builtin.Params.ABI, "set", thor.KeyMaxBlockProposers, big.NewInt(31))
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	// add initial authority nodes
	for _, anode := range initialAuthorityNodes {
		data := mustEncodeInput(builtin.Authority.ABI, "add", anode.masterAddress, endorsor, anode.identity)
		builder.Call(tx.NewClause(&builtin.Authority.Address).WithData(data), executor)
	}

	var extra [28]byte
	copy(extra[:], "193 TestNet")
	builder.ExtraData(extra)
	id, err := builder.ComputeID()
	if err != nil {
		panic(err)
	}
	return &Genesis{builder, id, "193"}
}

func load193AuthorityNodes() []*struct {
	masterAddress thor.Address
	identity      thor.Bytes32
} {
	all := [...][2]string{
		{"0xb8a1d772433926e93c5fd8d54e430f031b5e6e4d", "0x0000000000000000000000000000000000000000000000000000000000000030"},
		{"0x7960441df1e90956465e8850fdb82c206b962f33", "0x0000000000000000000000000000000000000000000000000000000000000031"},
		{"0x5695e8212518f3f890d13fde16c5e219d6c7f8df", "0x0000000000000000000000000000000000000000000000000000000000000032"},
		{"0xb682b36ca3bcbac7286110c9c41e039c5f5889e8", "0x0000000000000000000000000000000000000000000000000000000000000033"},
		{"0xc1a8801e5e2511375522c6911365c822e277b8cf", "0x0000000000000000000000000000000000000000000000000000000000000034"},
		{"0xcbc276f17657ec99fd011cdd9497f3c052fd49d7", "0x0000000000000000000000000000000000000000000000000000000000000035"},
		{"0x78503bcd09850a86862edc9300afd142ea8e0294", "0x0000000000000000000000000000000000000000000000000000000000000036"},
		{"0xc5ad4a86462b09a6642d86325ac8f3eecb8783a7", "0x0000000000000000000000000000000000000000000000000000000000000037"},
		{"0x72eb0273097df22a42ee50d7c6e39fc3eff88437", "0x0000000000000000000000000000000000000000000000000000000000000038"},
		{"0x3b4e58d11c4791e7c64485b714608dd32c50eaa3", "0x0000000000000000000000000000000000000000000000000000000000000039"},
		{"0x781f8f3483800933143a7d295cc516707b5739f1", "0x0000000000000000000000000000000000000000000000000000000000003130"},
		{"0x98db243fd4311bc2c7462f6a326c2b157cd925a6", "0x0000000000000000000000000000000000000000000000000000000000003131"},
		{"0xe8545c106559327585fbcc6692e61c4886b544f8", "0x0000000000000000000000000000000000000000000000000000000000003132"},
		{"0x418646ceea8dec36da5fe681cdad2cf66593b073", "0x0000000000000000000000000000000000000000000000000000000000003133"},
		{"0x8fcfbfa454be54db9f7f8bd46663df77321ee115", "0x0000000000000000000000000000000000000000000000000000000000003134"},
		{"0x966f8e85d64ac959238d0a454121ec12a00cef04", "0x0000000000000000000000000000000000000000000000000000000000003135"},
		{"0x131383b8e1c1cccf64a70ef54da4c314b9a1787a", "0x0000000000000000000000000000000000000000000000000000000000003136"},
		{"0xbe6569195d12e58834fcb98586acd4451656bf75", "0x0000000000000000000000000000000000000000000000000000000000003137"},
		{"0xa72bb00a561b0953b3fb253c70c8ffa079d311ca", "0x0000000000000000000000000000000000000000000000000000000000003138"},
		{"0x9894f36624c0751070208abb1552a11a568a81fb", "0x0000000000000000000000000000000000000000000000000000000000003139"},
		{"0x0f7529ad768efdd6ce01c1525174c9b8063a7f41", "0x0000000000000000000000000000000000000000000000000000000000003230"},
		{"0x06e493bd6ab5a307cdf5442fc1ed9bbe66d2a8aa", "0x0000000000000000000000000000000000000000000000000000000000003231"},
		{"0x00d4f03b7bc4a5b2337c7ca26295f4048a1f7a4f", "0x0000000000000000000000000000000000000000000000000000000000003232"},
		{"0x7ae4409f764e2cddb67ac20ba4ba14ad4a2eb364", "0x0000000000000000000000000000000000000000000000000000000000003233"},
		{"0x9c12fcaeeb9566cd5cef74ca7bc7a4b58ef027f9", "0x0000000000000000000000000000000000000000000000000000000000003234"},
		{"0xdcf766a117ce1463a46335cf2bda5aa755bf6166", "0x0000000000000000000000000000000000000000000000000000000000003235"},
		{"0xfb5ca899c3723dcf5092d04f1c180fec3d23b01e", "0x0000000000000000000000000000000000000000000000000000000000003236"},
		{"0x4d6fad287e09c4991a27aee1895b81853abbb289", "0x0000000000000000000000000000000000000000000000000000000000003237"},
		{"0xfb3788b9438d71eb0a85e5fd39a4bd655b48f020", "0x0000000000000000000000000000000000000000000000000000000000003238"},
		{"0x0d74de281aaf1189a90f5109c2e54ec488c639d4", "0x0000000000000000000000000000000000000000000000000000000000003239"},
		{"0xdb7972380557291f3222edbd5f6de81ce8c47051", "0x0000000000000000000000000000000000000000000000000000000000003330"},
	}

	candidates := make([]*struct {
		masterAddress thor.Address
		identity      thor.Bytes32
	}, 0, len(all))
	for _, item := range all {
		candidates = append(candidates, &struct {
			masterAddress thor.Address
			identity      thor.Bytes32
		}{
			masterAddress: thor.MustParseAddress(item[0]),
			identity:      thor.MustParseBytes32(item[1]),
		})
	}
	return candidates
}

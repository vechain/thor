// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"crypto/ecdsa"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// DevAccount account for development.
type DevAccount struct {
	Address    thor.Address
	PrivateKey *ecdsa.PrivateKey
}

var devAccounts atomic.Value

// DevAccounts returns pre-alloced accounts for solo mode.
func DevAccounts() []DevAccount {
	if accs := devAccounts.Load(); accs != nil {
		return accs.([]DevAccount)
	}

	var accs []DevAccount
	privKeys := []string{
		"99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36",
		"7b067f53d350f1cf20ec13df416b7b73e88a1dc7331bc904b92108b1e76a08b1",
		"f4a1a17039216f535d42ec23732c79943ffb45a089fbb78a14daad0dae93e991",
		"35b5cc144faca7d7f220fca7ad3420090861d5231d80eb23e1013426847371c4",
		"10c851d8d6c6ed9e6f625742063f292f4cf57c2dbeea8099fa3aca53ef90aef1",
		"2dd2c5b5d65913214783a6bd5679d8c6ef29ca9f2e2eae98b4add061d0b85ea0",
		"e1b72a1761ae189c10ec3783dd124b902ffd8c6b93cd9ff443d5490ce70047ff",
		"35cbc5ac0c3a2de0eb4f230ced958fd6a6c19ed36b5d2b1803a9f11978f96072",
		"b639c258292096306d2f60bc1a8da9bc434ad37f15cd44ee9a2526685f592220",
		"9d68178cdc934178cca0a0051f40ed46be153cf23cb1805b59cc612c0ad2bbe0",
	}
	for _, str := range privKeys {
		pk, err := crypto.HexToECDSA(str)
		if err != nil {
			panic(err)
		}
		addr := crypto.PubkeyToAddress(pk.PublicKey)
		accs = append(accs, DevAccount{thor.Address(addr), pk})
	}
	devAccounts.Store(accs)
	return accs
}

// NewDevnet create genesis for solo mode.
func NewDevnet() *Genesis {
	launchTime := uint64(1526400000) // 'Wed May 16 2018 00:00:00 GMT+0800 (CST)'

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

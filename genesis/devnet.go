// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"crypto/ecdsa"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/vechain/thor/v2/thor"
)

// Config for the devnet network, to be extended by our needs
type DevConfig struct {
	ForkConfig   *thor.ForkConfig
	BaseGasPrice *big.Int
	LaunchTime   uint64
	Config       *thor.Config
}

// DefaultHayabusaTP is the default Hayabusa transition period (0 = immediate PoS).
var DefaultHayabusaTP = uint32(0)

// SoloConfig is the default configuration for solo/dev mode.
// Uses short staking periods for faster testing.
var SoloConfig = thor.Config{
	LowStakingPeriod:    12,
	MediumStakingPeriod: 30,
	HighStakingPeriod:   90,
	CooldownPeriod:      12,
	HayabusaTP:          &DefaultHayabusaTP,
}

const (
	// DefaultDevnetLaunchTime is the default genesis timestamp for devnet.
	// Wed May 16 2018 00:00:00 GMT+0800 (CST)
	DefaultDevnetLaunchTime = uint64(1526400000)

	// InitialDevAccountBalance is 1 billion VET in wei (1e27).
	InitialDevAccountBalance = "1000000000000000000000000000"
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

func NewDevnet() (*Genesis, *thor.ForkConfig) {
	forkConfig := thor.SoloFork
	return NewDevnetWithConfig(DevConfig{
		ForkConfig: &forkConfig,
		Config:     &SoloConfig,
	}), &forkConfig
}

func NewDevnetWithConfig(config DevConfig) *Genesis {
	var gene CustomGenesis

	if config.ForkConfig != nil {
		gene.ForkConfig = config.ForkConfig
	} else {
		fc := thor.SoloFork
		gene.ForkConfig = &fc
	}

	if config.LaunchTime != 0 {
		gene.LaunchTime = config.LaunchTime
	} else {
		gene.LaunchTime = DefaultDevnetLaunchTime
	}

	if config.Config != nil {
		gene.Config = config.Config
	}

	if config.BaseGasPrice != nil {
		gene.Params.BaseGasPrice = (*HexOrDecimal256)(config.BaseGasPrice)
	}

	gene.Params.ExecutorAddress = &DevAccounts()[0].Address

	bal, _ := new(big.Int).SetString(InitialDevAccountBalance, 10)
	for _, a := range DevAccounts() {
		gene.Accounts = append(gene.Accounts, Account{
			Address: a.Address,
			Balance: (*HexOrDecimal256)(bal),
			Energy:  (*HexOrDecimal256)(bal),
		})
	}

	if gene.ForkConfig.HAYABUSA == 0 {
		gene.Stakers = append(gene.Stakers, Validator{
			Master:   DevAccounts()[0].Address,
			Endorser: DevAccounts()[0].Address,
		})
	} else {
		gene.Authority = append(gene.Authority, Authority{
			MasterAddress:   DevAccounts()[0].Address,
			EndorsorAddress: DevAccounts()[0].Address,
			Identity:        thor.BytesToBytes32([]byte("Solo Block Signer")),
		})
	}

	genesis, err := NewCustomNetWithName(&gene, "devnet")
	if err != nil {
		panic(err)
	}
	return genesis
}

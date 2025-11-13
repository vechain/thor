// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package testchain

import (
	"fmt"
	"math/big"
	"time"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
)

// CreateGenesis create a genesis with the given parameters
func CreateGenesis(config genesis.DevConfig, mbp uint64, epochLength uint32, transitionPeriod uint32) (*genesis.Genesis, error) {
	if mbp > uint64(len(genesis.DevAccounts())) {
		return nil, fmt.Errorf("max block proposers %d exceeds number of dev accounts %d", mbp, len(genesis.DevAccounts()))
	}
	amount := new(big.Int)
	amount.SetString("1000000000000000000000000000", 10)
	largeNo := (*genesis.HexOrDecimal256)(amount)
	// Create accounts for all DevAccounts
	accounts := make([]genesis.Account, len(genesis.DevAccounts()))
	for i, devAcc := range genesis.DevAccounts() {
		accounts[i] = genesis.Account{
			Address: devAcc.Address,
			Balance: largeNo,
			Energy:  largeNo,
		}
	}

	authorities := make([]genesis.Authority, mbp)
	for i := range mbp {
		authorities[i] = genesis.Authority{
			MasterAddress:   genesis.DevAccounts()[i].Address,
			EndorsorAddress: genesis.DevAccounts()[i].Address,
			Identity:        thor.BytesToBytes32([]byte("Block Signer")),
		}
	}

	if config.LaunchTime == 0 {
		now := uint64(time.Now().Unix())
		config.LaunchTime = now - now%thor.BlockInterval()
	}

	gen := &genesis.CustomGenesis{
		LaunchTime: config.LaunchTime,
		GasLimit:   40_000_000,
		ExtraData:  "packer test",
		Accounts:   accounts,
		Authority:  authorities,
		Params: genesis.Params{
			ExecutorAddress:   &genesis.DevAccounts()[0].Address,
			MaxBlockProposers: &mbp,
		},
		ForkConfig: config.ForkConfig,
	}

	if thor.HayabusaTP() != transitionPeriod || thor.EpochLength() != epochLength {
		gen.Config = &thor.Config{
			LowStakingPeriod:      epochLength,
			MediumStakingPeriod:   epochLength * 2,
			HighStakingPeriod:     epochLength * 3,
			CooldownPeriod:        epochLength,
			EvictionCheckInterval: epochLength,
			EpochLength:           epochLength,
			HayabusaTP:            &transitionPeriod,
		}
	}

	if config.KeyBaseGasPrice == nil || config.KeyBaseGasPrice.Sign() == 0 {
		gen.Params.BaseGasPrice = (*genesis.HexOrDecimal256)(config.KeyBaseGasPrice)
	}

	customNet, err := genesis.NewCustomNet(gen)
	if err != nil {
		return nil, err
	}
	return customNet, nil
}

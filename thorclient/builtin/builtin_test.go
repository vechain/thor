// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/builtin/staker"

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/test/testnode"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
)

func txContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func txOpts() *bind.TxOptions {
	gas := uint64(10_000_000)
	return &bind.TxOptions{
		Gas: &gas,
	}
}

func newRange(receipt *api.Receipt) *api.Range {
	block64 := uint64(receipt.Meta.BlockNumber)
	return &api.Range{
		From: &block64,
		To:   &block64,
	}
}

// newTestNode creates a node with the API enabled to test the smart contract wrappers
func newTestNode(t *testing.T, useExecutor bool) (testnode.Node, *thorclient.Client) {
	accounts := genesis.DevAccounts()
	authAccs := make([]genesis.Authority, 0, len(accounts))
	stateAccs := make([]genesis.Account, 0, len(accounts))

	for i, acc := range accounts {
		if i == 0 {
			authAccs = append(authAccs, genesis.Authority{
				MasterAddress:   acc.Address,
				EndorsorAddress: acc.Address,
				Identity:        thor.BytesToBytes32([]byte("master")),
			})
		}
		bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
		stateAccs = append(stateAccs, genesis.Account{
			Address: acc.Address,
			Balance: (*genesis.HexOrDecimal256)(bal),
			Energy:  (*genesis.HexOrDecimal256)(bal),
			Code:    "",
			Storage: nil,
		})
	}

	mbp := uint64(1_000)
	genConfig := genesis.CustomGenesis{
		LaunchTime: uint64(time.Now().Unix()),
		GasLimit:   thor.InitialGasLimit,
		ExtraData:  "",
		ForkConfig: &thor.SoloFork,
		Authority:  authAccs,
		Accounts:   stateAccs,
		Params: genesis.Params{
			MaxBlockProposers: &mbp,
		},
	}

	if useExecutor {
		approvers := make([]genesis.Approver, 3)
		for i, acc := range genesis.DevAccounts()[0:3] {
			approvers[i] = genesis.Approver{
				Address:  acc.Address,
				Identity: datagen.RandomHash(),
			}
		}
		genConfig.Executor = genesis.Executor{
			Approvers: approvers,
		}
	} else {
		genConfig.Params.ExecutorAddress = &genesis.DevAccounts()[0].Address
	}

	gene, err := genesis.NewCustomNet(&genConfig)
	require.NoError(t, err, "failed to create genesis builder")

	staker.LowStakingPeriod = solidity.NewConfigVariable("staker-low-staking-period", 360*24*7)
	staker.MediumStakingPeriod = solidity.NewConfigVariable("staker-medium-staking-period", 360*24*15)
	staker.HighStakingPeriod = solidity.NewConfigVariable("staker-high-staking-period", 360*24*30)
	staker.CooldownPeriod = solidity.NewConfigVariable("cooldown-period", 8640)
	staker.EpochLength = solidity.NewConfigVariable("epoch-length", 180)

	chain, err := testchain.NewIntegrationTestChainWithGenesis(gene, &thor.SoloFork, 180)
	if err != nil {
		t.Fatalf("failed to create test chain: %v", err)
	}

	chain = testchain.New(
		chain.Database(),
		chain.Genesis(),
		chain.Engine(),
		chain.Repo(),
		chain.Stater(),
		chain.GenesisBlock(),
		chain.LogDB(),
		chain.GetForkConfig(),
	)

	if err := chain.MintBlock(genesis.DevAccounts()[0]); err != nil {
		require.NoErrorf(t, err, "failed to mint genesis block")
	}

	node, err := testnode.NewNodeBuilder().WithChain(chain).Build()
	require.NoError(t, err)
	require.NoError(t, node.Start())

	return node, thorclient.New(node.APIServer().URL)
}

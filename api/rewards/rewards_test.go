// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rewards

import (
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"testing"

	"github.com/vechain/thor/v2/api"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

func initRewardsServer(t *testing.T) *httptest.Server {
	forkConfig := thor.NoFork
	forkConfig.GALACTICA = 0
	forkConfig.HAYABUSA = 0
	forkConfig.HAYABUSA_TP = 1
	thorChain, err := testchain.NewWithFork(&forkConfig)
	require.NoError(t, err)

	router := mux.NewRouter()
	rewards := New(thorChain.Repo(), thorChain.Engine(), thorChain.Stater(), &forkConfig)
	rewards.Mount(router, "/rewards")

	staker := thorChain.Contract(builtin.Staker.Address, builtin.Staker.ABI, genesis.DevAccounts()[0])
	err = staker.MintTransaction("addValidator", big.NewInt(0).Mul(big.NewInt(25000000), big.NewInt(1e18)), genesis.DevAccounts()[0].Address, uint32(360)*24*7)
	require.NoError(t, err)

	params := thorChain.Contract(builtin.Params.Address, builtin.Params.ABI, genesis.DevAccounts()[0])
	err = params.MintTransaction("set", big.NewInt(0), thor.KeyMaxBlockProposers, big.NewInt(1))
	assert.NoError(t, err)
	err = params.MintTransaction("set", big.NewInt(0), thor.KeyCurveFactor, thor.InitialCurveFactor)
	assert.NoError(t, err)

	require.NoError(t, thorChain.MintBlock(genesis.DevAccounts()[0]))

	return httptest.NewServer(router)
}

func TestGetBlockRewards(t *testing.T) {
	ts := initRewardsServer(t)
	t.Cleanup(ts.Close)

	tclient := thorclient.New(ts.URL)

	t.Run("getBlockRewardsBestBlock", func(t *testing.T) {
		res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/rewards/best")
		require.NoError(t, err)
		require.Equal(t, 200, statusCode)
		require.NotNil(t, res)

		var blockReward api.JSONBlockReward
		err = json.Unmarshal(res, &blockReward)
		require.NoError(t, err)

		assert.NotNil(t, blockReward.Reward)
		assert.NotNil(t, blockReward.Master)
		assert.NotNil(t, blockReward.ValidatorID)
	})

	t.Run("getBlockRewardsInvalidRevision", func(t *testing.T) {
		res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/rewards/invalid")
		require.NoError(t, err)
		require.Equal(t, 400, statusCode)
		require.NotNil(t, res)
		assert.Contains(t, string(res), "invalid syntax")
	})

	t.Run("getBlockRewardsNonExistentBlock", func(t *testing.T) {
		res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/rewards/999999")
		require.NoError(t, err)
		require.Equal(t, 400, statusCode)
		require.NotNil(t, res)
		assert.Contains(t, string(res), "revision: not found")
	})

	t.Run("getBlockRewardsPreHayabusa", func(t *testing.T) {
		res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/rewards/0")
		require.NoError(t, err)
		require.Equal(t, 400, statusCode)
		require.NotNil(t, res)
		assert.Contains(t, string(res), "pre hayabusa block")
	})
}

func TestBlockRewardCalculation(t *testing.T) {
	ts := initRewardsServer(t)
	t.Cleanup(ts.Close)

	tclient := thorclient.New(ts.URL)

	t.Run("calculateBlockReward", func(t *testing.T) {
		res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/rewards/best")
		require.NoError(t, err)
		require.Equal(t, 200, statusCode)

		var blockReward api.JSONBlockReward
		err = json.Unmarshal(res, &blockReward)
		require.NoError(t, err)

		assert.NotNil(t, blockReward.Reward)
		reward := (*big.Int)(blockReward.Reward)
		expected, _ := big.NewInt(0).SetString("121765601217656012176", 10)
		assert.Equal(t, expected, reward)
	})
}

func TestRewardResponseFormat(t *testing.T) {
	ts := initRewardsServer(t)
	t.Cleanup(ts.Close)

	tclient := thorclient.New(ts.URL)

	t.Run("verifyResponseStructure", func(t *testing.T) {
		res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/rewards/best")
		require.NoError(t, err)
		require.Equal(t, 200, statusCode)

		var blockReward api.JSONBlockReward
		err = json.Unmarshal(res, &blockReward)
		require.NoError(t, err)

		assert.NotNil(t, blockReward.Reward)
		assert.NotNil(t, blockReward.Master)
		assert.NotNil(t, blockReward.ValidatorID)

		reward := (*big.Int)(blockReward.Reward)
		assert.True(t, reward.Sign() >= 0, "reward should be non-negative")

		assert.Equal(t, 20, len(*blockReward.Master), "master address should be 20 bytes")

		assert.Equal(t, 20, len(*blockReward.ValidatorID), "validator ID should be 20 bytes")
	})
}

// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bind

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/bindcontract"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/test/testsolo"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

// testEnv holds the test environment including chain and contract
type testEnv struct {
	t *testing.T
	//chain        *testchain.Chain
	client       *thorclient.Client
	bindContract Contract
	owner        *PrivateKeySigner
	user         *PrivateKeySigner
	testSolo     *testsolo.Solo
}

// setupTestEnv creates a new test environment with a deployed test contract
func setupTestEnv(t *testing.T) *testEnv {
	// Create test chain
	forks := testchain.DefaultForkConfig
	forks.GALACTICA = 1
	thorChain, err := testchain.NewWithFork(&forks)
	require.NoError(t, err)

	// mint a block to arrive to Galactica
	if err := thorChain.MintBlock(genesis.DevAccounts()[0]); err != nil {
		require.NoErrorf(t, err, "failed to mint genesis block")
	}

	testSolo, err := testsolo.NewSolo(thorChain)
	require.NoError(t, err)

	// Get test accounts
	accounts := genesis.DevAccounts()
	owner := NewSigner(accounts[0].PrivateKey)
	user := NewSigner(accounts[1].PrivateKey)

	client := thorclient.New(testSolo.APIServer.URL)
	// Deploy test contract
	bindContract, err := DeployContract(client, owner, []byte(bindcontract.ABI), bindcontract.HexBytecode)
	require.NoError(t, err)

	return &testEnv{
		t:            t,
		testSolo:     testSolo,
		client:       client,
		bindContract: bindContract,
		owner:        owner,
		user:         user,
	}
}

func TestContract_Call(t *testing.T) {
	env := setupTestEnv(t)

	t.Run("GetValue", func(t *testing.T) {
		var value *big.Int
		err := env.bindContract.Method("getValue").
			Call().
			ExecuteInto(&value)
		require.NoError(t, err)
		assert.Equal(t, uint64(42), value.Uint64())
	})

	t.Run("GetOwner", func(t *testing.T) {
		out := common.Address{}
		err := env.bindContract.Method("getOwner").
			Call().
			ExecuteInto(&out)
		require.NoError(t, err)
		assert.Equal(t, env.owner.Address(), thor.Address(out))
	})
}

func TestContract_Send(t *testing.T) {
	env := setupTestEnv(t)

	t.Run("SetValue", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		receipt, _, err := env.bindContract.Method("setValue", big.NewInt(100)).
			Send().
			WithSigner(env.owner).
			WithOptions(&TxOptions{Gas: ptr(uint64(100000))}).
			SubmitAndConfirm(ctx)
		require.NoError(t, err)
		assert.False(t, receipt.Reverted)

		// Verify the value was changed
		var value *big.Int
		err = env.bindContract.Method("getValue").
			Call().
			ExecuteInto(&value)
		require.NoError(t, err)
		assert.Equal(t, uint64(100), value.Uint64())
	})

	t.Run("TransferOwnership", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Try as non-owner (should fail)
		receipt, _, err := env.bindContract.Method("transferOwnership", env.user.Address()).
			Send().
			WithSigner(env.user).
			WithOptions(&TxOptions{Gas: ptr(uint64(100000))}).
			SubmitAndConfirm(ctx)
		require.NoError(t, err)
		assert.True(t, receipt.Reverted)

		// Try as owner (should succeed)
		receipt, _, err = env.bindContract.Method("transferOwnership", env.user.Address()).
			Send().
			WithSigner(env.owner).
			WithOptions(&TxOptions{Gas: ptr(uint64(100000))}).
			SubmitAndConfirm(ctx)
		require.NoError(t, err)
		assert.False(t, receipt.Reverted)

		// Verify ownership was transferred
		out := common.Address{}
		err = env.bindContract.Method("getOwner").
			Call().
			ExecuteInto(&out)
		require.NoError(t, err)
		assert.Equal(t, env.user.Address(), thor.Address(out))
	})
}

func TestContract_Filter(t *testing.T) {
	env := setupTestEnv(t)

	t.Run("ValueChanged", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Set a new value to trigger the event
		receipt, _, err := env.bindContract.Method("setValue", big.NewInt(200)).
			Send().
			WithSigner(env.owner).
			WithOptions(&TxOptions{Gas: ptr(uint64(100000))}).
			SubmitAndConfirm(ctx)
		require.NoError(t, err)
		assert.False(t, receipt.Reverted)

		// Filter events
		events, err := env.bindContract.FilterEvent("ValueChanged").
			InRange(&events.Range{
				From: ptr(uint64(receipt.Meta.BlockNumber)),
				To:   ptr(uint64(receipt.Meta.BlockNumber)),
			}).
			Execute()
		require.NoError(t, err)
		assert.Len(t, events, 1)

		// Verify event data
		event := events[0]
		assert.Equal(t, env.bindContract.Address(), &event.Address)
		assert.Len(t, event.Topics, 2) // event signature + oldValue

		// Get the event definition from ABI
		eventDef, ok := env.bindContract.ABI().Events["ValueChanged"]
		require.True(t, ok, "event ValueChanged not found in ABI")

		// Decode the hex data
		data := make([]any, 1)
		data[0] = new(*big.Int)

		bytes, err := hexutil.Decode(event.Data)
		require.NoError(t, err)

		err = eventDef.Inputs.Unpack(&data, bytes)
		require.NoError(t, err)

		newValue := *(data[0].(**big.Int))
		assert.Equal(t, uint64(200), newValue.Uint64())
	})
}

// Helper function to create a pointer to uint64
func ptr(v uint64) *uint64 {
	return &v
}

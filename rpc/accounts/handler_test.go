// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package accounts_test

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/rpc/accounts"
	"github.com/vechain/thor/v2/rpc/testutil"
	"github.com/vechain/thor/v2/test/testchain"
)

type fixture struct {
	chain      *testchain.Chain
	senderAddr string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	c, err := testchain.NewDefault()
	require.NoError(t, err)

	chainID := c.Repo().ChainID()
	sender := genesis.DevAccounts()[0]
	recipient := genesis.DevAccounts()[1]

	ethTx := testutil.BuildEthTx(t, chainID, sender, 0, &recipient.Address)
	require.NoError(t, c.MintBlock(ethTx))

	return &fixture{
		chain:      c,
		senderAddr: sender.Address.String(),
	}
}

func TestAccountsHandler(t *testing.T) {
	fx := newFixture(t)
	ts := testutil.NewTestServer(t, accounts.New(fx.chain.Repo(), fx.chain.Stater()))

	t.Run("eth_getBalance", func(t *testing.T) {
		result := testutil.Call(t, ts, "eth_getBalance", []any{fx.senderAddr, "latest"})
		var bal hexutil.Big
		require.NoError(t, json.Unmarshal(result, &bal))
		assert.True(t, bal.ToInt().Sign() > 0, "funded dev account should have non-zero balance")
	})

	t.Run("eth_getCode_eoa", func(t *testing.T) {
		// EOAs have no code.
		result := testutil.Call(t, ts, "eth_getCode", []any{fx.senderAddr, "latest"})
		var code hexutil.Bytes
		require.NoError(t, json.Unmarshal(result, &code))
		assert.Empty(t, code)
	})

	t.Run("eth_getStorageAt_zero_slot", func(t *testing.T) {
		// Slot 0 of an EOA is always zero.
		result := testutil.Call(t, ts, "eth_getStorageAt", []any{fx.senderAddr, "0x0", "latest"})
		var slot common.Hash
		require.NoError(t, json.Unmarshal(result, &slot))
		assert.Equal(t, common.Hash{}, slot)
	})

	t.Run("eth_getTransactionCount_after_eth_tx", func(t *testing.T) {
		// The fixture sender sent one ETH tx with nonce 0; the runtime increments
		// the nonce to 1 and persists it in the committed trie.
		result := testutil.Call(t, ts, "eth_getTransactionCount", []any{fx.senderAddr, "latest"})
		var nonce hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &nonce))
		assert.Equal(t, uint64(1), uint64(nonce))
	})

	t.Run("eth_getTransactionCount_fresh_account", func(t *testing.T) {
		// An account that has never sent an ETH tx has nonce 0.
		freshAddr := genesis.DevAccounts()[5].Address.String()
		result := testutil.Call(t, ts, "eth_getTransactionCount", []any{freshAddr, "latest"})
		var nonce hexutil.Uint64
		require.NoError(t, json.Unmarshal(result, &nonce))
		assert.Equal(t, uint64(0), uint64(nonce))
	})
}

// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"bytes"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/comm/proto"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/p2p"
	"github.com/vechain/thor/v2/p2p/discover"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

func TestHandleRPC_MsgNewTx(t *testing.T) {
	chain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	repo := chain.Repo()

	pool := txpool.New(repo, chain.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	}, &thor.SoloFork)
	defer pool.Close()

	comm := New(repo, pool)
	peer := newPeer(p2p.NewPeer(discover.NodeID{}, "test", nil), stubMsgReadWriter{})

	to, _ := thor.ParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed")
	chainTag := repo.ChainTag()
	testTx := tx.NewBuilder(tx.TypeLegacy).
		ChainTag(chainTag).
		BlockRef(tx.NewBlockRef(0)).
		Expiration(100).
		GasPriceCoef(0).
		Gas(21000).
		Nonce(1).
		Clause(tx.NewClause(&to).WithValue(big.NewInt(1))).
		Build()
	testTx = tx.MustSign(testTx, genesis.DevAccounts()[0].PrivateKey)
	txHash := testTx.Hash()
	txID := testTx.ID()

	txData, err := rlp.EncodeToBytes(testTx)
	require.NoError(t, err)

	t.Run("valid transaction", func(t *testing.T) {
		writeCalled := false
		var writtenData interface{}

		write := func(data any) {
			writeCalled = true
			writtenData = data
		}

		msg := &p2p.Msg{
			Code:    proto.MsgNewTx,
			Size:    uint32(len(txData)),
			Payload: bytes.NewReader(txData),
		}

		txsToSync := &txsToSync{}

		err := comm.handleRPC(peer, msg, write, txsToSync)
		assert.NoError(t, err)

		assert.True(t, peer.IsTransactionKnown(txHash), "transaction should be marked on peer")

		addedTx := pool.Get(txID)
		assert.NotNil(t, addedTx, "transaction should be added to pool")
		if addedTx != nil {
			assert.Equal(t, testTx.Hash(), addedTx.Hash(), "added transaction should match")
		}

		assert.True(t, writeCalled, "write function should be called")
		assert.Equal(t, &struct{}{}, writtenData, "write should be called with empty struct")
	})

	t.Run("transaction exceeds size limit", func(t *testing.T) {
		writeCalled := false

		write := func(data any) {
			writeCalled = true
		}

		msg := &p2p.Msg{
			Code:    proto.MsgNewTx,
			Size:    maxTxSize + 1,
			Payload: bytes.NewReader(txData),
		}

		txsToSync := &txsToSync{}

		err := comm.handleRPC(peer, msg, write, txsToSync)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "payload size: exceeds limit")

		assert.False(t, writeCalled, "write should not be called on error")
	})

	t.Run("decode error", func(t *testing.T) {
		writeCalled := false

		write := func(data any) {
			writeCalled = true
		}

		invalidData := []byte{0x01, 0x02, 0x03}
		msg := &p2p.Msg{
			Code:    proto.MsgNewTx,
			Size:    uint32(len(invalidData)),
			Payload: bytes.NewReader(invalidData),
		}

		txsToSync := &txsToSync{}

		err := comm.handleRPC(peer, msg, write, txsToSync)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decode msg")

		assert.False(t, writeCalled, "write should not be called on error")
	})

	t.Run("transaction at size limit boundary", func(t *testing.T) {
		writeCalled := false
		var writtenData interface{}

		write := func(data any) {
			writeCalled = true
			writtenData = data
		}

		msg := &p2p.Msg{
			Code:    proto.MsgNewTx,
			Size:    maxTxSize,
			Payload: bytes.NewReader(txData),
		}

		txsToSync := &txsToSync{}

		err := comm.handleRPC(peer, msg, write, txsToSync)
		assert.NoError(t, err)

		assert.True(t, writeCalled, "write function should be called")
		assert.Equal(t, &struct{}{}, writtenData, "write should be called with empty struct")
	})
}

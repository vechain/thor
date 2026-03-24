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

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/comm/proto"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/p2p"
	"github.com/vechain/thor/v2/p2p/discover"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

func newTestCommunicator(t *testing.T) (*Communicator, *testchain.Chain) {
	t.Helper()
	chain, err := testchain.NewWithFork(&thor.SoloFork, 180)
	require.NoError(t, err)
	repo := chain.Repo()

	pool := txpool.New(repo, chain.Stater(), txpool.Options{
		Limit:           10000,
		LimitPerAccount: 16,
		MaxLifetime:     10 * time.Minute,
	}, &thor.SoloFork)
	t.Cleanup(pool.Close)

	return New(repo, pool), chain
}

func newTestPeer() *Peer {
	return newPeer(p2p.NewPeer(discover.NodeID{}, "test", nil), stubMsgReadWriter{})
}

func TestHandleRPC_MsgNewBlock(t *testing.T) {
	comm, chain := newTestCommunicator(t)
	require.NoError(t, chain.MintBlock())

	best := chain.Repo().BestBlockSummary()
	blk, err := chain.Repo().GetBlock(best.Header.ID())
	require.NoError(t, err)

	t.Run("valid block", func(t *testing.T) {
		peer := newTestPeer()
		blockData, err := rlp.EncodeToBytes(blk)
		require.NoError(t, err)

		blockCh := make(chan *NewBlockEvent, 1)
		sub := comm.SubscribeBlock(blockCh)
		defer sub.Unsubscribe()

		writeCalled := false
		write := func(data any) { writeCalled = true }

		msg := &p2p.Msg{
			Code:    proto.MsgNewBlock,
			Size:    uint32(len(blockData)),
			Payload: bytes.NewReader(blockData),
		}

		err = comm.handleRPC(peer, msg, write, &txsToSync{})
		require.NoError(t, err)

		assert.True(t, writeCalled)
		assert.True(t, peer.IsBlockKnown(blk.Header().ID()))
		_, score := peer.Head()
		assert.Equal(t, blk.Header().TotalScore(), score)

		select {
		case evt := <-blockCh:
			assert.Equal(t, blk.Header().ID(), evt.Block.Header().ID())
			assert.Equal(t, len(blk.Transactions()), len(evt.Transactions()))
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for block event")
		}
	})

	t.Run("block with transactions", func(t *testing.T) {
		peer := newTestPeer()

		to := thor.BytesToAddress([]byte("to"))
		trx := tx.MustSign(
			tx.NewBuilder(tx.TypeLegacy).
				ChainTag(chain.Repo().ChainTag()).
				BlockRef(tx.NewBlockRef(0)).
				Expiration(100).
				Gas(21000).
				Nonce(99).
				Clause(tx.NewClause(&to).WithValue(big.NewInt(1))).
				Build(),
			genesis.DevAccounts()[0].PrivateKey,
		)
		require.NoError(t, chain.MintBlock(trx))

		blkWithTx, err := chain.Repo().GetBlock(chain.Repo().BestBlockSummary().Header.ID())
		require.NoError(t, err)
		require.NotEmpty(t, blkWithTx.Transactions())

		blockData, err := rlp.EncodeToBytes(blkWithTx)
		require.NoError(t, err)

		blockCh := make(chan *NewBlockEvent, 1)
		sub := comm.SubscribeBlock(blockCh)
		defer sub.Unsubscribe()

		writeCalled := false
		write := func(data any) { writeCalled = true }

		msg := &p2p.Msg{
			Code:    proto.MsgNewBlock,
			Size:    uint32(len(blockData)),
			Payload: bytes.NewReader(blockData),
		}

		err = comm.handleRPC(peer, msg, write, &txsToSync{})
		require.NoError(t, err)

		assert.True(t, writeCalled)

		select {
		case evt := <-blockCh:
			assert.Equal(t, blkWithTx.Header().ID(), evt.Block.Header().ID())
			assert.Equal(t, len(blkWithTx.Transactions()), len(evt.Transactions()))
			for i, origTx := range blkWithTx.Transactions() {
				assert.Equal(t, origTx.ID(), evt.Block.Transactions()[i].ID())
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for block event")
		}
	})

	t.Run("decode error with invalid payload", func(t *testing.T) {
		peer := newTestPeer()
		invalidData := []byte{0x01, 0x02, 0x03}

		writeCalled := false
		write := func(data any) { writeCalled = true }

		msg := &p2p.Msg{
			Code:    proto.MsgNewBlock,
			Size:    uint32(len(invalidData)),
			Payload: bytes.NewReader(invalidData),
		}

		err := comm.handleRPC(peer, msg, write, &txsToSync{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decode msg")
		assert.False(t, writeCalled)
	})

	t.Run("decode error with extra fields in block", func(t *testing.T) {
		peer := newTestPeer()
		data, err := rlp.EncodeToBytes([]any{
			blk.Header(),
			blk.Transactions(),
			[]byte("extra"),
		})
		require.NoError(t, err)

		writeCalled := false
		write := func(data any) { writeCalled = true }

		msg := &p2p.Msg{
			Code:    proto.MsgNewBlock,
			Size:    uint32(len(data)),
			Payload: bytes.NewReader(data),
		}

		err = comm.handleRPC(peer, msg, write, &txsToSync{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decode msg")
		assert.False(t, writeCalled)
	})

	t.Run("two-phase decode matches single-phase", func(t *testing.T) {
		blockData, err := rlp.EncodeToBytes(blk)
		require.NoError(t, err)

		var directBlock block.Block
		require.NoError(t, rlp.DecodeBytes(blockData, &directBlock))

		var rawBlk block.RawBlock
		require.NoError(t, rlp.DecodeBytes(blockData, &rawBlk))
		twoPhaseBlock, err := rawBlk.Decode()
		require.NoError(t, err)

		assert.Equal(t, directBlock.Header().ID(), twoPhaseBlock.Header().ID())
		assert.Equal(t, directBlock.Header().TotalScore(), twoPhaseBlock.Header().TotalScore())
		assert.Equal(t, len(directBlock.Transactions()), len(twoPhaseBlock.Transactions()))
	})
}

func TestHandleRPC_MsgNewTx(t *testing.T) {
	comm, chain := newTestCommunicator(t)
	repo := chain.Repo()
	peer := newTestPeer()

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
		var writtenData any

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

		addedTx := comm.txPool.Get(txID)
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
		var writtenData any

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

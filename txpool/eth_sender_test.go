// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
)

func ethTxObjectForSenderTest(
	t *testing.T,
	tchain *testchain.Chain,
	sender genesis.DevAccount,
	nonce uint64,
	maxPriorityFeePerGas int64,
	maxFeePerGas int64,
) *TxObject {
	t.Helper()
	txObj, err := ResolveTx(
		buildEthTxForChain(t, tchain, sender, nonce, maxPriorityFeePerGas, maxFeePerGas),
		false,
	)
	require.NoError(t, err)
	return txObj
}

func TestEthSenderPlacePromotesContiguousQueue(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[0]
	base := int64(thor.InitialBaseFee)
	state := newEthSender(0)

	tx2 := ethTxObjectForSenderTest(t, tchain, sender, 2, base, 2*base)
	replaced, accepted := state.place(tx2)
	require.True(t, accepted)
	assert.Nil(t, replaced)
	assert.Equal(t, uint64(0), state.nextPendingNonce())
	assert.Contains(t, state.queue, uint64(2))

	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	replaced, accepted = state.place(tx0)
	require.True(t, accepted)
	assert.Nil(t, replaced)
	assert.Equal(t, uint64(1), state.nextPendingNonce())

	tx1 := ethTxObjectForSenderTest(t, tchain, sender, 1, base, 2*base)
	replaced, accepted = state.place(tx1)
	require.True(t, accepted)
	assert.Nil(t, replaced)
	assert.Equal(t, uint64(3), state.nextPendingNonce())
	assert.Empty(t, state.queue)

	pending := state.sortedPending()
	require.Len(t, pending, 3)
	assert.Equal(t, uint64(0), pending[0].Nonce())
	assert.Equal(t, uint64(1), pending[1].Nonce())
	assert.Equal(t, uint64(2), pending[2].Nonce())
}

func TestEthSenderReplacementAndDropDemotion(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[1]
	base := int64(thor.InitialBaseFee)
	state := newEthSender(0)

	nonce0 := ethTxObjectForSenderTest(t, tchain, sender, 0, 2*base, 4*base)
	_, accepted := state.place(nonce0)
	require.True(t, accepted)

	lower := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 3*base)
	replaced, accepted := state.place(lower)
	assert.False(t, accepted)
	assert.Nil(t, replaced)
	assert.Equal(t, nonce0.ID(), state.pending[0].ID())

	higher := ethTxObjectForSenderTest(t, tchain, sender, 0, 3*base, 5*base)
	replaced, accepted = state.place(higher)
	require.True(t, accepted)
	assert.Equal(t, nonce0.ID(), replaced.ID())
	assert.Equal(t, higher.ID(), state.pending[0].ID())

	_, accepted = state.place(ethTxObjectForSenderTest(t, tchain, sender, 1, base, 2*base))
	require.True(t, accepted)
	nonce2 := ethTxObjectForSenderTest(t, tchain, sender, 2, base, 2*base)
	_, accepted = state.place(nonce2)
	require.True(t, accepted)

	dropped := state.dropNonce(1)
	require.NotNil(t, dropped)
	assert.NotContains(t, state.pending, uint64(1))
	assert.NotContains(t, state.pending, uint64(2))
	assert.Contains(t, state.queue, uint64(2), "trailing pending nonce must be demoted")
	assert.False(t, state.empty())
}

func TestEthSenderBumpStateNonceEvictsAndPromotes(t *testing.T) {
	_, tchain := newEthPoolTestPool(t)
	sender := genesis.DevAccounts()[2]
	base := int64(thor.InitialBaseFee)
	state := newEthSender(0)

	tx0 := ethTxObjectForSenderTest(t, tchain, sender, 0, base, 2*base)
	tx2 := ethTxObjectForSenderTest(t, tchain, sender, 2, base, 2*base)
	_, accepted := state.place(tx0)
	require.True(t, accepted)
	_, accepted = state.place(tx2)
	require.True(t, accepted)

	evicted := state.bumpStateNonce(2)
	require.Len(t, evicted, 1)
	assert.Equal(t, tx0.ID(), evicted[0].ID())
	assert.Equal(t, uint64(3), state.nextPendingNonce())
	assert.Empty(t, state.queue)
	assert.Equal(t, tx2.ID(), state.pending[2].ID())
}

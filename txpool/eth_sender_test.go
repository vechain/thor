// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func feeTx(feeCap, tipCap int64) *TxObject {
	return &TxObject{Transaction: tx.NewBuilder(tx.TypeEthDynamicFee).
		MaxFeePerGas(big.NewInt(feeCap)).
		MaxPriorityFeePerGas(big.NewInt(tipCap)).
		Build()}
}

func TestEthFeeBump(t *testing.T) {
	incumbent := feeTx(100, 10)

	assert.False(t, isFeeBumpSufficient(incumbent, feeTx(109, 11), 10), "fee cap is below 10% bump")
	assert.False(t, isFeeBumpSufficient(incumbent, feeTx(110, 10), 10), "tip must strictly increase")
	assert.True(t, isFeeBumpSufficient(incumbent, feeTx(110, 11), 10))

	low := feeTx(1, 1)
	assert.False(t, isFeeBumpSufficient(low, feeTx(1, 1), 10), "integer rounding must not allow equality")
	assert.True(t, isFeeBumpSufficient(low, feeTx(2, 2), 10))
}

func TestEthSenderPoolNonceAndStateSync(t *testing.T) {
	sender := newEthSender(thor.Address{1}, 4)
	sender.pending[4] = feeTx(10, 1)
	sender.pending[5] = feeTx(10, 1)
	sender.queue[8] = feeTx(10, 1)

	assert.Equal(t, uint64(6), sender.poolNonce())
	settled, releases := sender.syncStateNonce(5)
	assert.Len(t, settled, 1)
	assert.Len(t, releases, 1)
	assert.Equal(t, uint64(5), sender.stateNonce)
	assert.NotNil(t, sender.pending[5])
	assert.NotNil(t, sender.queue[8])
}

func TestEthSenderSyncStateNonceBackward(t *testing.T) {
	origin := thor.Address{2}
	sender := newEthSender(origin, 6)
	tx6, tx7 := feeTx(10, 1), feeTx(10, 1)
	tx6.executable, tx7.executable = true, true
	sender.pending[6], sender.pending[7] = tx6, tx7

	settled, releases := sender.syncStateNonce(4)
	assert.Empty(t, settled)
	assert.Len(t, releases, 2)
	assert.Equal(t, uint64(4), sender.poolNonce())
	assert.Empty(t, sender.pending)
	assert.Same(t, tx6, sender.queue[6])
	assert.Same(t, tx7, sender.queue[7])
	assert.False(t, tx6.executable)
	assert.False(t, tx7.executable)
}

func TestEthSenderResetStateNonceRevalidatesUnchangedNonce(t *testing.T) {
	origin := thor.Address{3}
	sender := newEthSender(origin, 4)
	tx4 := feeTx(10, 1)
	tx4.executable = true
	sender.pending[4] = tx4

	settled, releases := sender.resetStateNonce(4)
	assert.Empty(t, settled)
	assert.Len(t, releases, 1)
	assert.Empty(t, sender.pending)
	assert.Same(t, tx4, sender.queue[4])
	assert.False(t, tx4.executable)
	assert.Equal(t, uint64(4), sender.poolNonce())
}

func TestEthSenderDropNonce(t *testing.T) {
	origin := thor.Address{4}
	sender := newEthSender(origin, 10)
	tx10, tx11, tx12 := feeTx(10, 1), feeTx(10, 1), feeTx(10, 1)
	queued := feeTx(10, 1)
	for nonce, txObj := range map[uint64]*TxObject{
		10: tx10,
		11: tx11,
		12: tx12,
	} {
		txObj.executable = true
		sender.pending[nonce] = txObj
	}
	sender.queue[14] = queued

	releases, dropped := sender.dropNonce(11)

	assert.True(t, dropped)
	assert.ElementsMatch(t, []reservationOwner{
		ethReservationOwner(origin, 11),
		ethReservationOwner(origin, 12),
	}, releases)
	assert.Same(t, tx10, sender.pending[10])
	assert.NotContains(t, sender.pending, uint64(11))
	assert.NotContains(t, sender.queue, uint64(11), "the dropped transaction must not be retained")
	assert.Same(t, tx12, sender.queue[12])
	assert.Same(t, queued, sender.queue[14])
	assert.False(t, tx11.executable)
	assert.False(t, tx12.executable)
	assert.Equal(t, uint64(11), sender.poolNonce())

	releases, dropped = sender.dropNonce(13)
	assert.False(t, dropped)
	assert.Nil(t, releases)
	assert.Same(t, tx10, sender.pending[10])
	assert.Same(t, tx12, sender.queue[12])
}

func TestEthSenderDemoteFrom(t *testing.T) {
	origin := thor.Address{5}
	sender := newEthSender(origin, 20)
	tx20, tx21, tx22 := feeTx(10, 1), feeTx(10, 1), feeTx(10, 1)
	for nonce, txObj := range map[uint64]*TxObject{
		20: tx20,
		21: tx21,
		22: tx22,
	} {
		txObj.executable = true
		sender.pending[nonce] = txObj
	}

	releases := sender.demoteFrom(21)

	assert.ElementsMatch(t, []reservationOwner{
		ethReservationOwner(origin, 21),
		ethReservationOwner(origin, 22),
	}, releases)
	assert.Same(t, tx20, sender.pending[20])
	assert.Same(t, tx21, sender.queue[21])
	assert.Same(t, tx22, sender.queue[22])
	assert.False(t, tx21.executable)
	assert.False(t, tx22.executable)
	assert.Equal(t, uint64(21), sender.poolNonce())

	assert.Nil(t, sender.demoteFrom(23))
	assert.Same(t, tx20, sender.pending[20])
	assert.Same(t, tx21, sender.queue[21])
	assert.Same(t, tx22, sender.queue[22])
}

func TestEthSenderPendingCountFrom(t *testing.T) {
	sender := newEthSender(thor.Address{6}, 7)
	sender.pending[7] = feeTx(10, 1)
	sender.pending[8] = feeTx(10, 1)
	sender.pending[9] = feeTx(10, 1)
	sender.queue[10] = feeTx(10, 1)

	assert.Equal(t, 3, sender.pendingCountFrom(0))
	assert.Equal(t, 3, sender.pendingCountFrom(7))
	assert.Equal(t, 2, sender.pendingCountFrom(8))
	assert.Equal(t, 1, sender.pendingCountFrom(9))
	assert.Zero(t, sender.pendingCountFrom(10), "queued transactions must not be counted")
	assert.Zero(t, sender.pendingCountFrom(math.MaxUint64))
}

func TestEthSenderDropNonceMaxUint64DoesNotWrap(t *testing.T) {
	origin := thor.Address{7}
	sender := newEthSender(origin, math.MaxUint64)
	maxNonceTx := feeTx(10, 1)
	lowerNonceSentinel := feeTx(10, 1)
	maxNonceTx.executable = true
	lowerNonceSentinel.executable = true
	sender.pending[math.MaxUint64] = maxNonceTx
	sender.pending[0] = lowerNonceSentinel

	releases, dropped := sender.dropNonce(math.MaxUint64)

	assert.True(t, dropped)
	assert.Equal(t, []reservationOwner{
		ethReservationOwner(origin, math.MaxUint64),
	}, releases)
	assert.NotContains(t, sender.pending, uint64(math.MaxUint64))
	assert.NotContains(t, sender.queue, uint64(math.MaxUint64))
	assert.False(t, maxNonceTx.executable)
	assert.Same(t, lowerNonceSentinel, sender.pending[0],
		"a wrapped demotion boundary would incorrectly move lower nonces")
	assert.NotContains(t, sender.queue, uint64(0))
	assert.True(t, lowerNonceSentinel.executable)
}

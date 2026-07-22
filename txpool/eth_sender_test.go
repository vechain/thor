// Copyright (c) 2026 The VeChainThor developers

package txpool

import (
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

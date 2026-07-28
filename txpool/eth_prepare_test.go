// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareEthObjectsDedupes(t *testing.T) {
	txObj := newEthCoreTestObject(t, 0, 10, 0)
	calls := 0
	prepared := prepareEthObjects([]*TxObject{txObj, txObj}, func(obj *TxObject) ethPreparation {
		calls++
		return fixedEthPrepare(1, 100)(obj)
	})

	assert.Equal(t, 1, calls)
	assert.Len(t, prepared.byTx, 1)
}

func TestEthPreparationsGetUsesCache(t *testing.T) {
	txObj := newEthCoreTestObject(t, 0, 10, 0)
	calls := 0
	prepared := prepareEthObjects([]*TxObject{txObj}, func(obj *TxObject) ethPreparation {
		calls++
		return fixedEthPrepare(1, 100)(obj)
	})

	for range 3 {
		preparation, err := prepared.get(txObj)
		require.NoError(t, err)
		assert.True(t, preparation.viable)
	}
	assert.Equal(t, 1, calls, "cached preparations must not re-read chain state")
}

func TestEthPreparationsGetFallsBackInline(t *testing.T) {
	covered := newEthCoreTestObject(t, 0, 10, 0)
	missing := newEthCoreTestObject(t, 1, 10, 0)
	calls := 0
	prepared := prepareEthObjects([]*TxObject{covered}, func(obj *TxObject) ethPreparation {
		calls++
		return fixedEthPrepare(1, 100)(obj)
	})
	require.Equal(t, 1, calls)

	preparation, err := prepared.get(missing)
	require.NoError(t, err)
	assert.True(t, preparation.viable)
	assert.Equal(t, 2, calls)

	// The fallback result is cached, so a second miss costs nothing.
	_, err = prepared.get(missing)
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestEthPreparationsGetWithoutPrepareReportsMissing(t *testing.T) {
	txObj := newEthCoreTestObject(t, 0, 10, 0)

	_, err := ethPreparations{}.get(txObj)
	assert.ErrorIs(t, err, errEthPreparationMissing)
}

// An empty preparation set must still commit correctly by preparing inline,
// so a too-narrow preparation window can never reject a valid transaction.
func TestEthPoolCorePromotesWithoutPrePass(t *testing.T) {
	m := newEthPoolCore(newCostTracker())
	first := newEthCoreTestObject(t, 0, 10, 0)
	second := newEthCoreTestObject(t, 1, 10, 0)
	origin := first.Origin()
	sender := newEthSender(origin, 0)
	sender.queue[0] = first
	sender.queue[1] = second
	m.senders[origin] = sender
	m.allByHash[first.Hash()] = first
	m.allByHash[second.Hash()] = second

	m.lock.Lock()
	promoted, err := m.promoteLocked(sender, 16, prepareEthObjects(nil, fixedEthPrepare(1, 100)))
	m.lock.Unlock()

	require.NoError(t, err)
	assert.Equal(t, []*TxObject{first, second}, promoted)
	assert.Len(t, sender.pending, 2)
	assert.Empty(t, sender.queue)
}

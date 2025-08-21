// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package staker

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func TestTransition(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	param := params.New(thor.BytesToAddress([]byte("params")), st)

	assert.NoError(t, param.Set(thor.KeyMaxBlockProposers, big.NewInt(2)))
	stakerAddr := thor.BytesToAddress([]byte("stkr"))
	staker := New(stakerAddr, st, param, nil)

	isExecuted, err := staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.False(t, isExecuted)

	node1 := datagen.RandAddress()
	stake := RandomStake()
	err = staker.AddValidation(node1, node1, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	isExecuted, err = staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.False(t, isExecuted)

	node2 := datagen.RandAddress()
	err = staker.AddValidation(node2, node2, uint32(360)*24*15, stake)
	assert.NoError(t, err)

	staker.params.Set(thor.KeyMaxBlockProposers, big.NewInt(0))

	isExecuted, err = staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.False(t, isExecuted)

	staker.params.Set(thor.KeyMaxBlockProposers, big.NewInt(2))

	isExecuted, err = staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.True(t, isExecuted)

	isExecuted, err = staker.transition(thor.EpochLength())
	assert.NoError(t, err)
	assert.False(t, isExecuted)

	activeCountSlot := thor.BytesToBytes32([]byte(("validations-active-group-size")))
	st.SetRawStorage(stakerAddr, activeCountSlot, rlp.RawValue{0xFF})
	isExecuted, err = staker.transition(thor.EpochLength())
	assert.Error(t, err)
	assert.False(t, isExecuted)

	queuedCountSlot := thor.BytesToBytes32([]byte(("validations-queued-group-size")))
	st.SetRawStorage(stakerAddr, activeCountSlot, rlp.RawValue{0x0})
	st.SetRawStorage(stakerAddr, queuedCountSlot, rlp.RawValue{0xFF})
}

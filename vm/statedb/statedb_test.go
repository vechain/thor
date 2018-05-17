// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package statedb_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	. "github.com/vechain/thor/vm/statedb"
)

func TestStateSnapshot(t *testing.T) {
	assert := assert.New(t)
	kv, _ := lvldb.NewMem()
	state, _ := state.New(thor.Bytes32{}, kv)

	statedb := New(state)

	addrOne := common.Address{1}
	addrTwo := common.Address{2}

	////
	ver := statedb.Snapshot()
	assert.Equal(ver, 1)

	statedb.AddBalance(addrOne, big.NewInt(10))
	statedb.AddBalance(addrTwo, big.NewInt(20))
	statedb.SetState(addrOne, common.Hash{10}, common.Hash{10})

	////
	ver = statedb.Snapshot()
	assert.Equal(ver, 2)

	statedb.AddBalance(addrOne, big.NewInt(20))
	statedb.AddBalance(addrTwo, big.NewInt(30))
	statedb.SetState(addrOne, common.Hash{10}, common.Hash{20})

	assert.Equal(statedb.GetBalance(addrOne), big.NewInt(30))
	assert.Equal(statedb.GetBalance(addrTwo), big.NewInt(50))
	assert.Equal(statedb.GetState(addrOne, common.Hash{10}), common.Hash{20})

	////
	statedb.RevertToSnapshot(ver)

	assert.Equal(statedb.GetBalance(addrOne), big.NewInt(10))
	assert.Equal(statedb.GetBalance(addrTwo), big.NewInt(20))
	assert.Equal(statedb.GetState(addrOne, common.Hash{10}), common.Hash{10})

	////
	ver = statedb.Snapshot()
	assert.Equal(2, ver)

}

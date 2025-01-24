// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"github.com/vechain/thor/v2/trie"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

func M(a ...interface{}) []interface{} {
	return a
}

func TestStaker(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc := thor.BytesToAddress([]byte("a1"))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)
	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{M(stkr.TotalStake()), M(&big.Int{}, nil)},
		{st.SetBalance(acc, big.NewInt(12)), nil},
		{stkr.Stake(acc, big.NewInt(10)), nil},
		{st.SetBalance(acc, big.NewInt(2)), nil},
		{st.SetBalance(stkr.addr, big.NewInt(10)), nil},
		{M(stkr.GetStake(acc)), M(big.NewInt(10), nil)},
		{stkr.Unstake(acc, big.NewInt(3)), nil},
		{M(stkr.GetStake(acc)), M(big.NewInt(7), nil)},
		{M(st.GetBalance(acc)), M(big.NewInt(5), nil)},
		{M(st.GetBalance(stkr.addr)), M(big.NewInt(7), nil)},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}
}

func TestTotalStaked(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	balance, err := st.GetBalance(stkr.addr)
	assert.Nil(t, err)
	assert.Equal(t, balance, big.NewInt(0))
}

func TestStake(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc := thor.BytesToAddress([]byte("a1"))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc, big.NewInt(11))

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	err = stkr.Stake(acc, big.NewInt(10))
	assert.Nil(t, err)

	newStake, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, newStake, big.NewInt(10))

	afterStaking, err := stkr.GetStake(thor.BytesToAddress([]byte("a1")))

	assert.Nil(t, err)
	assert.Equal(t, afterStaking, big.NewInt(10))
}

func TestStakeInsufficient(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc := thor.BytesToAddress([]byte("a1"))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc, big.NewInt(2))

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	err = stkr.Stake(acc, big.NewInt(10))
	assert.Equal(t, "insufficient balance: address balance 2 is less than amount 10", err.Error())
}

func TestUnstakeInsufficientTotalStaked(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc := thor.BytesToAddress([]byte("a1"))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc, big.NewInt(5))

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	err = stkr.Stake(acc, big.NewInt(5))

	err = stkr.Unstake(acc, big.NewInt(6))
	assert.Equal(t, "insufficient total staked: total staked 5 is less than amount 6", err.Error())
}

func TestUnstakeInsufficientBalance(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc1 := thor.BytesToAddress([]byte("a1"))
	acc2 := thor.BytesToAddress([]byte("a2"))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc1, big.NewInt(5))
	err = st.SetBalance(acc2, big.NewInt(10))

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	st.SetBalance(stkr.addr, big.NewInt(6))

	err = stkr.Stake(acc1, big.NewInt(5))
	err = stkr.Stake(acc2, big.NewInt(3))

	err = stkr.Unstake(acc1, big.NewInt(6))
	assert.Equal(t, "insufficient stake: account stake 5 is less than amount 6", err.Error())
}

func TestUnstakeInsufficientContractBalance(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc1 := thor.BytesToAddress([]byte("a1"))
	acc2 := thor.BytesToAddress([]byte("a2"))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc1, big.NewInt(5))
	err = st.SetBalance(acc2, big.NewInt(10))

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	st.SetBalance(stkr.addr, big.NewInt(4))

	err = stkr.Stake(acc1, big.NewInt(5))
	err = stkr.Stake(acc2, big.NewInt(3))

	err = stkr.Unstake(acc1, big.NewInt(5))
	assert.Equal(t, "insufficient balance: contract balance 4 is less than amount 5", err.Error())
}

func TestUnstake(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc := thor.BytesToAddress([]byte("a1"))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)
	err := st.SetBalance(acc, big.NewInt(11))

	err = stkr.Stake(acc, big.NewInt(10))
	assert.Nil(t, err)

	err = st.SetBalance(acc, big.NewInt(1))
	err = st.SetBalance(stkr.addr, big.NewInt(10))

	newStake, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, newStake, big.NewInt(10))

	afterStaking, err := stkr.GetStake(thor.BytesToAddress([]byte("a1")))
	assert.Nil(t, err)
	assert.Equal(t, afterStaking, big.NewInt(10))

	accBalance, err := st.GetBalance(acc)
	assert.Nil(t, err)
	assert.Equal(t, accBalance, big.NewInt(1))

	conBalance, err := st.GetBalance(stkr.addr)
	assert.Nil(t, err)
	assert.Equal(t, conBalance, big.NewInt(10))

	err = stkr.Unstake(thor.BytesToAddress([]byte("a1")), big.NewInt(5))
	assert.Nil(t, err)

	newStake, err = stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, newStake, big.NewInt(5))

	afterUnStaking, err := stkr.GetStake(thor.BytesToAddress([]byte("a1")))
	assert.Nil(t, err)
	assert.Equal(t, afterUnStaking, big.NewInt(5))

	accBalance, err = st.GetBalance(acc)
	assert.Nil(t, err)
	assert.Equal(t, accBalance, big.NewInt(6))

	conBalance, err = st.GetBalance(stkr.addr)
	assert.Nil(t, err)
	assert.Equal(t, conBalance, big.NewInt(5))
}

func TestGetStake(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc1 := thor.BytesToAddress([]byte("a1"))
	acc2 := thor.BytesToAddress([]byte("a2"))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc1, big.NewInt(11))
	err = st.SetBalance(acc2, big.NewInt(3))

	// get initial stake should return 0
	staked1, err := stkr.GetStake(acc1)
	assert.Nil(t, err)
	assert.Equal(t, staked1, big.NewInt(0))

	staked2, err := stkr.GetStake(acc2)
	assert.Nil(t, err)
	assert.Equal(t, staked2, big.NewInt(0))

	err = stkr.Stake(acc1, big.NewInt(7))
	assert.Nil(t, err)

	err = stkr.Stake(acc2, big.NewInt(3))
	assert.Nil(t, err)

	staked1, err = stkr.GetStake(acc1)
	assert.Nil(t, err)
	assert.Equal(t, staked1, big.NewInt(7))

	staked2, err = stkr.GetStake(acc2)
	assert.Nil(t, err)
	assert.Equal(t, staked2, big.NewInt(3))
}

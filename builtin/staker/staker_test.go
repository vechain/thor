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
	validatorAcc := thor.BytesToAddress([]byte("v1"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)
	tests := []struct {
		ret      interface{}
		expected interface{}
	}{
		{M(stkr.TotalStake()), M(&big.Int{}, nil)},
		{st.SetBalance(validatorAcc, stakeAmount), nil},
		{stkr.AddValidator(stakeAmount, validatorAcc), nil},
		{st.SetBalance(stkr.addr, stakeAmount), nil},
		{M(stkr.TotalStake()), M(stakeAmount, nil)},
		{st.SetBalance(acc, big.NewInt(12)), nil},
		{stkr.Stake(acc, validatorAcc, big.NewInt(10)), nil},
		{st.SetBalance(stkr.addr, big.NewInt(0).Add(stakeAmount, big.NewInt(10))), nil},
		{M(stkr.TotalStake()), M(big.NewInt(0).Add(stakeAmount, big.NewInt(10)), nil)},
		{st.SetBalance(acc, big.NewInt(2)), nil},
		{st.SetBalance(stkr.addr, big.NewInt(10)), nil},
		{M(stkr.GetStake(acc, validatorAcc)), M(big.NewInt(10), nil)},
		{stkr.Unstake(acc, big.NewInt(3), validatorAcc), nil},
		{M(stkr.GetStake(acc, validatorAcc)), M(big.NewInt(7), nil)},
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
	acc1 := thor.BytesToAddress([]byte("a1"))
	acc2 := thor.BytesToAddress([]byte("a2"))
	validatorAcc1 := thor.BytesToAddress([]byte("v1"))
	validatorAcc2 := thor.BytesToAddress([]byte("v2"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc1, big.NewInt(11))
	err = st.SetBalance(acc2, big.NewInt(3))
	err = st.SetBalance(validatorAcc1, stakeAmount)
	err = st.SetBalance(validatorAcc2, stakeAmount)

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	err = stkr.AddValidator(stakeAmount, validatorAcc1)
	err = stkr.AddValidator(stakeAmount, validatorAcc2)

	err = stkr.Stake(acc1, validatorAcc1, big.NewInt(7))
	assert.Nil(t, err)

	err = stkr.Stake(acc2, validatorAcc2, big.NewInt(3))
	assert.Nil(t, err)

	st.SetBalance(stkr.addr, big.NewInt(0).Add(big.NewInt(10), big.NewInt(0).Mul(stakeAmount, big.NewInt(2))))
	staked, err = stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0).Add(big.NewInt(10), big.NewInt(0).Mul(stakeAmount, big.NewInt(2))))
}

func TestStake(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	validatorAcc := thor.BytesToAddress([]byte("v1"))
	acc := thor.BytesToAddress([]byte("a1"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc, big.NewInt(11))
	err = st.SetBalance(validatorAcc, stakeAmount)

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	err = stkr.AddValidator(stakeAmount, validatorAcc)
	err = stkr.Stake(acc, validatorAcc, big.NewInt(10))
	assert.Nil(t, err)

	st.SetBalance(stkr.addr, big.NewInt(0).Add(stakeAmount, big.NewInt(10)))
	newStake, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, newStake, big.NewInt(0).Add(stakeAmount, big.NewInt(10)))

	afterStaking, err := stkr.GetStake(thor.BytesToAddress([]byte("a1")), validatorAcc)

	assert.Nil(t, err)
	assert.Equal(t, afterStaking, big.NewInt(10))
}

func TestUnstakeValidator(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc := thor.BytesToAddress([]byte("a1"))
	validatorAcc := thor.BytesToAddress([]byte("v1"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc, big.NewInt(5))
	err = st.SetBalance(validatorAcc, stakeAmount)

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	err = stkr.AddValidator(stakeAmount, validatorAcc)

	err = stkr.Unstake(validatorAcc, big.NewInt(0).Add(stakeAmount, big.NewInt(1)), validatorAcc)
	assert.Equal(t, "validator cannot unstake from itself", err.Error())
}

func TestStakeNonExistingValidator(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc := thor.BytesToAddress([]byte("a1"))
	validatorAcc1 := thor.BytesToAddress([]byte("v1"))
	validatorAcc2 := thor.BytesToAddress([]byte("v2"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc, big.NewInt(10))

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	err = stkr.Stake(acc, validatorAcc2, big.NewInt(10))
	assert.Equal(t, "validator 0x0000000000000000000000000000000000007632 does not exist", err.Error())

	st.SetBalance(validatorAcc1, stakeAmount)
	err = stkr.AddValidator(stakeAmount, validatorAcc1)
	assert.Nil(t, err)

	err = stkr.Stake(acc, validatorAcc2, big.NewInt(10))
	assert.Equal(t, "validator 0x0000000000000000000000000000000000007632 does not exist", err.Error())
}

func TestUnstakeInsufficientBalance(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc1 := thor.BytesToAddress([]byte("a1"))
	acc2 := thor.BytesToAddress([]byte("a2"))
	validatorAcc1 := thor.BytesToAddress([]byte("v1"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc1, big.NewInt(5))
	err = st.SetBalance(acc2, big.NewInt(10))
	err = st.SetBalance(validatorAcc1, stakeAmount)

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	st.SetBalance(stkr.addr, big.NewInt(6))
	err = stkr.AddValidator(stakeAmount, validatorAcc1)

	err = stkr.Stake(acc1, validatorAcc1, big.NewInt(5))
	err = stkr.Stake(acc2, validatorAcc1, big.NewInt(3))

	err = stkr.Unstake(acc1, big.NewInt(6), validatorAcc1)
	assert.Equal(t, "insufficient stake: account stake 5 is less than amount 6", err.Error())
}

func TestUnstakeInsufficientContractBalance(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc1 := thor.BytesToAddress([]byte("a1"))
	acc2 := thor.BytesToAddress([]byte("a2"))
	validatorAcc1 := thor.BytesToAddress([]byte("v1"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc1, big.NewInt(5))
	err = st.SetBalance(acc2, big.NewInt(10))
	err = st.SetBalance(validatorAcc1, stakeAmount)

	// get initial supply before set should return 0
	staked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, staked, big.NewInt(0))

	st.SetBalance(stkr.addr, big.NewInt(4))
	err = stkr.AddValidator(stakeAmount, validatorAcc1)

	err = stkr.Stake(acc1, validatorAcc1, big.NewInt(5))
	err = stkr.Stake(acc2, validatorAcc1, big.NewInt(3))

	st.SetBalance(stkr.addr, big.NewInt(4))
	err = stkr.Unstake(acc1, big.NewInt(5), validatorAcc1)
	assert.Equal(t, "insufficient total staked: total staked 4 is less than amount 5", err.Error())
}

func TestUnstake(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc := thor.BytesToAddress([]byte("a1"))
	validatorAcc1 := thor.BytesToAddress([]byte("v1"))
	validatorAcc2 := thor.BytesToAddress([]byte("v2"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)
	err := st.SetBalance(acc, big.NewInt(11))
	err = st.SetBalance(validatorAcc1, stakeAmount)
	err = st.SetBalance(validatorAcc2, stakeAmount)

	err = stkr.AddValidator(stakeAmount, validatorAcc1)
	err = stkr.AddValidator(stakeAmount, validatorAcc2)
	err = stkr.Stake(acc, validatorAcc1, big.NewInt(10))
	assert.Nil(t, err)

	err = st.SetBalance(acc, big.NewInt(1))
	err = st.SetBalance(stkr.addr, big.NewInt(0).Add(big.NewInt(10), big.NewInt(0).Mul(stakeAmount, big.NewInt(2))))

	newStake, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, newStake, big.NewInt(0).Add(big.NewInt(0).Mul(big.NewInt(2), stakeAmount), big.NewInt(10)))

	afterStaking, err := stkr.GetStake(thor.BytesToAddress([]byte("a1")), validatorAcc1)
	assert.Nil(t, err)
	assert.Equal(t, afterStaking, big.NewInt(10))

	accBalance, err := st.GetBalance(acc)
	assert.Nil(t, err)
	assert.Equal(t, accBalance, big.NewInt(1))

	conBalance, err := st.GetBalance(stkr.addr)
	assert.Nil(t, err)
	assert.Equal(t, conBalance, big.NewInt(0).Add(big.NewInt(0).Mul(big.NewInt(2), stakeAmount), big.NewInt(10)))

	err = stkr.Unstake(acc, big.NewInt(5), validatorAcc2)
	assert.Equal(t, "insufficient stake: account stake 0 is less than amount 5", err.Error())

	err = stkr.Unstake(acc, big.NewInt(5), validatorAcc1)
	assert.Nil(t, err)

	newStake, err = stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, newStake, big.NewInt(0).Add(big.NewInt(0).Mul(big.NewInt(2), stakeAmount), big.NewInt(5)))

	afterUnStaking, err := stkr.GetStake(thor.BytesToAddress([]byte("a1")), validatorAcc1)
	assert.Nil(t, err)
	assert.Equal(t, afterUnStaking, big.NewInt(5))

	accBalance, err = st.GetBalance(acc)
	assert.Nil(t, err)
	assert.Equal(t, accBalance, big.NewInt(6))

	conBalance, err = st.GetBalance(stkr.addr)
	assert.Nil(t, err)
	assert.Equal(t, conBalance, big.NewInt(0).Add(big.NewInt(0).Mul(big.NewInt(2), stakeAmount), big.NewInt(5)))
}

func TestGetStake(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	acc1 := thor.BytesToAddress([]byte("a1"))
	acc2 := thor.BytesToAddress([]byte("a2"))
	validatorAcc1 := thor.BytesToAddress([]byte("v1"))
	validatorAcc2 := thor.BytesToAddress([]byte("v2"))
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	err := st.SetBalance(acc1, big.NewInt(11))
	err = st.SetBalance(acc2, big.NewInt(3))
	err = st.SetBalance(validatorAcc1, stakeAmount)
	err = st.SetBalance(validatorAcc2, stakeAmount)

	// get initial stake should return 0
	staked1, err := stkr.GetStake(acc1, validatorAcc1)
	assert.Nil(t, err)
	assert.Equal(t, staked1, big.NewInt(0))

	staked2, err := stkr.GetStake(acc1, validatorAcc2)
	assert.Nil(t, err)
	assert.Equal(t, staked2, big.NewInt(0))

	staked3, err := stkr.GetStake(acc2, validatorAcc1)
	assert.Nil(t, err)
	assert.Equal(t, staked3, big.NewInt(0))

	staked4, err := stkr.GetStake(acc2, validatorAcc2)
	assert.Nil(t, err)
	assert.Equal(t, staked4, big.NewInt(0))

	err = stkr.AddValidator(stakeAmount, validatorAcc1)
	err = stkr.AddValidator(stakeAmount, validatorAcc2)
	err = stkr.Stake(acc1, validatorAcc1, big.NewInt(7))
	assert.Nil(t, err)

	err = stkr.Stake(acc2, validatorAcc2, big.NewInt(3))
	assert.Nil(t, err)

	staked1, err = stkr.GetStake(acc1, validatorAcc1)
	assert.Nil(t, err)
	assert.Equal(t, staked1, big.NewInt(7))

	staked2, err = stkr.GetStake(acc1, validatorAcc2)
	assert.Nil(t, err)
	assert.Equal(t, staked2, big.NewInt(0))

	staked3, err = stkr.GetStake(acc2, validatorAcc2)
	assert.Nil(t, err)
	assert.Equal(t, staked3, big.NewInt(3))

	staked4, err = stkr.GetStake(acc2, validatorAcc1)
	assert.Nil(t, err)
	assert.Equal(t, staked4, big.NewInt(0))
}

func TestAddValidator(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	validatorAcc1 := thor.BytesToAddress([]byte("v1"))
	validatorAcc2 := thor.BytesToAddress([]byte("v2"))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	validators, err := stkr.ListValidators()
	assert.Nil(t, err)
	assert.Equal(t, len(validators), 0)

	st.SetBalance(validatorAcc1, stakeAmount)
	err = stkr.AddValidator(stakeAmount, validatorAcc1)
	assert.Nil(t, err)
	validators, err = stkr.ListValidators()
	assert.Nil(t, err)
	assert.Equal(t, len(validators), 1)
	assert.Equal(t, validators[0], validatorAcc1)

	err = stkr.AddValidator(big.NewInt(1000000000), validatorAcc2)
	assert.Equal(t, err.Error(), "amount is less than minimum stake")

	st.SetBalance(validatorAcc2, stakeAmount)
	err = stkr.AddValidator(stakeAmount, validatorAcc2)
	assert.Nil(t, err)

	validators, err = stkr.ListValidators()
	assert.Nil(t, err)
	assert.Equal(t, len(validators), 2)
	assert.Equal(t, validators[0], validatorAcc1)
	assert.Equal(t, validators[1], validatorAcc2)

	err = stkr.AddValidator(stakeAmount, validatorAcc1)
	assert.Equal(t, err.Error(), "validator already exists")

	validators, err = stkr.ListValidators()
	assert.Nil(t, err)
	assert.Equal(t, len(validators), 2)
	assert.Equal(t, validators[0], validatorAcc1)
	assert.Equal(t, validators[1], validatorAcc2)
}

func TestRemoveValidator(t *testing.T) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	stakeAmount := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	validatorAcc1 := thor.BytesToAddress([]byte("v1"))
	validatorAcc2 := thor.BytesToAddress([]byte("v2"))
	validatorAcc3 := thor.BytesToAddress([]byte("v3"))
	acc1 := thor.BytesToAddress([]byte("a1"))
	acc2 := thor.BytesToAddress([]byte("a2"))

	stkr := New(thor.BytesToAddress([]byte("stkr")), st, 0)

	st.SetBalance(acc1, big.NewInt(7))
	st.SetBalance(acc2, big.NewInt(3))

	st.SetBalance(validatorAcc1, stakeAmount)
	err := stkr.AddValidator(stakeAmount, validatorAcc1)
	assert.Nil(t, err)

	stkr.Stake(acc1, validatorAcc1, big.NewInt(7))
	stkr.Stake(acc2, validatorAcc1, big.NewInt(3))
	st.SetBalance(stkr.addr, big.NewInt(0).Add(big.NewInt(10), stakeAmount))

	totalStaked, err := stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, totalStaked, big.NewInt(0).Add(stakeAmount, big.NewInt(10)))

	st.SetBalance(validatorAcc2, stakeAmount)
	err = stkr.AddValidator(stakeAmount, validatorAcc2)
	assert.Nil(t, err)

	st.SetBalance(validatorAcc3, stakeAmount)
	err = stkr.AddValidator(stakeAmount, validatorAcc3)
	assert.Nil(t, err)

	st.SetBalance(stkr.addr, big.NewInt(0).Add(big.NewInt(10), big.NewInt(0).Mul(stakeAmount, big.NewInt(3))))
	validators, err := stkr.ListValidators()
	assert.Nil(t, err)
	assert.Equal(t, len(validators), 3)
	assert.Equal(t, validators[0], validatorAcc1)
	assert.Equal(t, validators[1], validatorAcc2)
	assert.Equal(t, validators[2], validatorAcc3)

	err = stkr.RemoveValidator(validatorAcc1)
	assert.Nil(t, err)
	validators, err = stkr.ListValidators()
	assert.Nil(t, err)
	assert.Equal(t, len(validators), 2)
	assert.Equal(t, validators[0], validatorAcc2)
	assert.Equal(t, validators[1], validatorAcc3)

	balance, err := st.GetBalance(validatorAcc1)
	assert.Equal(t, stakeAmount, balance)

	totalStaked, err = stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, totalStaked, stakeAmount)

	err = stkr.RemoveValidator(validatorAcc1)
	assert.Equal(t, "validator does not exist", err.Error())

	validators, err = stkr.ListValidators()
	assert.Nil(t, err)
	assert.Equal(t, len(validators), 2)
	assert.Equal(t, validators[0], validatorAcc2)
	assert.Equal(t, validators[1], validatorAcc3)

	err = stkr.RemoveValidator(validatorAcc3)
	assert.Nil(t, err)
	validators, err = stkr.ListValidators()
	assert.Nil(t, err)
	assert.Equal(t, len(validators), 1)
	assert.Equal(t, validators[0], validatorAcc2)

	err = stkr.RemoveValidator(validatorAcc2)
	assert.Nil(t, err)
	validators, err = stkr.ListValidators()
	assert.Nil(t, err)
	assert.Equal(t, len(validators), 0)

	totalStaked, err = stkr.TotalStake()
	assert.Nil(t, err)
	assert.Equal(t, totalStaked.Int64(), big.NewInt(0).Int64())
}

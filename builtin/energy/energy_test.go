// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package energy

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func M(a ...any) []any {
	return a
}

func TestEnergy(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	acc := thor.BytesToAddress([]byte("a1"))

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(thor.BytesToAddress([]byte("eng")), st, 0, p)
	tests := []struct {
		ret      any
		expected any
	}{
		{M(eng.Get(acc)), M(&big.Int{}, nil)},
		{eng.Add(acc, big.NewInt(10)), nil},
		{M(eng.Get(acc)), M(big.NewInt(10), nil)},
		{M(eng.Sub(acc, big.NewInt(5))), M(true, nil)},
		{M(eng.Sub(acc, big.NewInt(6))), M(false, nil)},
		{eng.Add(acc, big.NewInt(0)), nil},
		{M(eng.Sub(acc, big.NewInt(0))), M(true, nil)},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.ret)
	}
}

func TestInitialSupply(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(thor.BytesToAddress([]byte("eng")), st, 0, p)

	// get initial supply before set should return 0
	supply, err := eng.getInitialSupply()
	assert.Nil(t, err)
	assert.Equal(t, supply, initialSupply{Token: big.NewInt(0), Energy: big.NewInt(0), BlockTime: 0x0})

	eng.SetInitialSupply(big.NewInt(123), big.NewInt(456))

	supply, err = eng.getInitialSupply()
	assert.Nil(t, err)
	assert.Equal(t, supply, initialSupply{Token: big.NewInt(123), Energy: big.NewInt(456), BlockTime: 0x0})
}

func TestInitialSupplyError(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(thor.BytesToAddress([]byte("a1")), st, 0, p)

	eng.SetInitialSupply(big.NewInt(0), big.NewInt(0))

	supply, err := eng.getInitialSupply()

	assert.Nil(t, err)
	assert.Equal(t, supply, initialSupply{Token: big.NewInt(0), Energy: big.NewInt(0), BlockTime: 0x0})
}

func TestTotalSupply(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	addr := thor.BytesToAddress([]byte("eng"))
	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(addr, st, 1, p)

	eng.SetInitialSupply(big.NewInt(100000000000000000), big.NewInt(456))

	totalSupply, err := eng.TotalSupply()

	assert.Nil(t, err)
	assert.Equal(t, big.NewInt(456), totalSupply)

	eng.blockTime = 1000
	totalSupply, err = eng.TotalSupply()

	assert.Nil(t, err)
	assert.Equal(t, big.NewInt(499500000456), totalSupply)

	eng.blockTime = 1500
	eng.StopEnergyGrowth()
	totalSupply, err = eng.TotalSupply()

	assert.Nil(t, err)
	assert.Equal(t, big.NewInt(749500000456), totalSupply)

	eng.blockTime = 2000
	totalSupply, err = eng.TotalSupply()

	assert.Nil(t, err)
	assert.Equal(t, big.NewInt(749500000456), totalSupply)

	eng.blockTime = 2001
	eng.addIssued(big.NewInt(100))
	totalSupply, err = eng.TotalSupply()

	assert.Nil(t, err)
	assert.Equal(t, big.NewInt(749500000556), totalSupply)

	eng.blockTime = 3000
	totalSupply, err = eng.TotalSupply()

	assert.Nil(t, err)
	assert.Equal(t, big.NewInt(749500000556), totalSupply)
}

func TestTokenTotalSupply(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(thor.BytesToAddress([]byte("eng")), st, 0, p)

	eng.SetInitialSupply(big.NewInt(123), big.NewInt(456))

	totalTokenSupply, err := eng.TokenTotalSupply()

	assert.Nil(t, err)
	assert.Equal(t, totalTokenSupply, big.NewInt(123))
}

func TestTotalBurned(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(thor.BytesToAddress([]byte("eng")), st, 0, p)

	eng.SetInitialSupply(big.NewInt(123), big.NewInt(456))

	totalBurned, err := eng.TotalBurned()

	assert.Nil(t, err)
	assert.Equal(t, totalBurned, big.NewInt(0))
}

func TestEnergyGrowth(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	acc := thor.BytesToAddress([]byte("a1"))

	st.SetEnergy(acc, &big.Int{}, 10)

	vetBal := big.NewInt(1e18)
	st.SetBalance(acc, vetBal)

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	bal1, err := New(thor.Address{}, st, 1000, p).
		Get(acc)

	assert.Nil(t, err)

	x := new(big.Int).Mul(thor.EnergyGrowthRate, vetBal)
	x.Mul(x, new(big.Int).SetUint64(1000-10))
	x.Div(x, big.NewInt(1e18))

	assert.Equal(t, x, bal1)
}

func TestGetEnergyGrowthStopTime(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})
	p := params.New(thor.BytesToAddress([]byte("params")), st)
	eng := New(thor.BytesToAddress([]byte("energy")), st, 10, p)

	// no stop time set
	stopTime, err := eng.GetEnergyGrowthStopTime()
	assert.NoError(t, err)
	assert.Equal(t, uint64(math.MaxUint64), stopTime)

	err = eng.StopEnergyGrowth()
	assert.NoError(t, err)

	stopTime, err = eng.GetEnergyGrowthStopTime()
	assert.NoError(t, err)
	assert.Equal(t, uint64(10), stopTime)

	// set multiple times should return error
	err = eng.StopEnergyGrowth()
	assert.Error(t, err, "energy growth has already stopped")
}

func TestAddIssued(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(thor.BytesToAddress([]byte("eng")), st, 1, p)

	issued, err := eng.getIssued()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), issued)

	storage, err := st.GetStorage(eng.addr, issuedKey)
	assert.NoError(t, err)
	assert.Equal(t, thor.Bytes32{}, storage)

	err = eng.addIssued(big.NewInt(100))
	assert.NoError(t, err)

	issued, err = eng.getIssued()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(100), issued)

	storage, err = st.GetStorage(eng.addr, issuedKey)
	assert.NoError(t, err)
	assert.Equal(t, thor.BytesToBytes32(big.NewInt(100).Bytes()), storage)
}

type mockStaker struct {
	lockedVET         *big.Int
	lockedWeight      *big.Int
	hasDelegations    bool
	increaseRewardErr error
}

func (m *mockStaker) LockedVET() (*big.Int, *big.Int, error) {
	return m.lockedVET, m.lockedWeight, nil
}

func (m *mockStaker) HasDelegations(address thor.Address) (bool, error) {
	return m.hasDelegations, nil
}

func (m *mockStaker) IncreaseReward(master thor.Address, reward big.Int) error {
	return m.increaseRewardErr
}

func TestCalculateRewards(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(thor.BytesToAddress([]byte("eng")), st, 1, p)

	stake := big.NewInt(0).Mul(big.NewInt(25), big.NewInt(1e18))

	mockStaker := &mockStaker{
		lockedVET:         stake,
		hasDelegations:    false,
		increaseRewardErr: nil,
	}

	reward, err := eng.CalculateRewards(mockStaker)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(121765601217656012), reward)

	leapEng := New(thor.BytesToAddress([]byte("eng")), st, 69638400, p)
	reward, err = leapEng.CalculateRewards(mockStaker)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(121432908318154219), reward)
}

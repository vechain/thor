// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package energy

import (
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	eng := New(thor.BytesToAddress([]byte("eng")), st, 0, p, nil)
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
	eng := New(thor.BytesToAddress([]byte("eng")), st, 0, p, nil)

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
	eng := New(thor.BytesToAddress([]byte("a1")), st, 0, p, nil)

	eng.SetInitialSupply(big.NewInt(0), big.NewInt(0))

	supply, err := eng.getInitialSupply()

	assert.Nil(t, err)
	assert.Equal(t, supply, initialSupply{Token: big.NewInt(0), Energy: big.NewInt(0), BlockTime: 0x0})
}

func TestTotalSupply(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	addr := thor.BytesToAddress([]byte("eng"))
	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(addr, st, 1, p, nil)

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
	eng := New(thor.BytesToAddress([]byte("eng")), st, 0, p, nil)

	eng.SetInitialSupply(big.NewInt(123), big.NewInt(456))

	totalTokenSupply, err := eng.TokenTotalSupply()

	assert.Nil(t, err)
	assert.Equal(t, totalTokenSupply, big.NewInt(123))
}

func TestTotalBurned(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(thor.BytesToAddress([]byte("eng")), st, 0, p, nil)

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
	bal1, err := New(thor.Address{}, st, 1000, p, nil).
		Get(acc)

	assert.Nil(t, err)

	x := new(big.Int).Mul(thor.EnergyGrowthRate, vetBal)
	x.Mul(x, new(big.Int).SetUint64(1000-10))
	x.Div(x, big.NewInt(1e18))

	assert.Equal(t, x, bal1)
}

func TestCalcEnergyCappedAtStopTime(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	acc := thor.BytesToAddress([]byte("a1"))

	st.SetEnergy(acc, &big.Int{}, 10)
	vetBal := big.NewInt(1e18)
	st.SetBalance(acc, vetBal)

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(thor.Address{}, st, 1000, p, nil)

	// Set stop time at 500
	eng.blockTime = 500

	bal, err := eng.Get(acc)
	assert.NoError(t, err)

	// growth should be (500 - 10) * rate * balance / 1e18
	expected := new(big.Int).Mul(thor.EnergyGrowthRate, vetBal)
	expected.Mul(expected, new(big.Int).SetUint64(500-10))
	expected.Div(expected, big.NewInt(1e18))
	assert.Equal(t, expected, bal)
}

func TestGetEnergyGrowthStopTime(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})
	p := params.New(thor.BytesToAddress([]byte("params")), st)
	eng := New(thor.BytesToAddress([]byte("energy")), st, 10, p, nil)

	// no stop time set
	stopTime, err := eng.GetEnergyGrowthStopTime()
	assert.NoError(t, err)
	assert.Equal(t, uint64(math.MaxUint64), stopTime)

	err = eng.StopEnergyGrowth()
	assert.NoError(t, err)

	stopTime, err = eng.GetEnergyGrowthStopTime()
	assert.NoError(t, err)
	assert.Equal(t, uint64(10), stopTime)

	// set multiple times should return nil
	err = eng.StopEnergyGrowth()
	assert.NoError(t, err)
}

func TestAddIssued(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(thor.BytesToAddress([]byte("eng")), st, 1, p, nil)

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
	lockedVET         uint64
	lockedWeight      uint64
	hasDelegations    bool
	increaseRewardErr error
	increases         map[thor.Address]*big.Int
}

func (m *mockStaker) LockedStake() (uint64, uint64, error) {
	return m.lockedVET, m.lockedWeight, nil
}

func (m *mockStaker) HasDelegations(address thor.Address) (bool, error) {
	return m.hasDelegations, nil
}

func (m *mockStaker) IncreaseDelegatorsReward(master thor.Address, reward *big.Int, currentBlock uint32) error {
	if m.increases == nil {
		m.increases = make(map[thor.Address]*big.Int)
	}
	if existing, ok := m.increases[master]; !ok {
		m.increases[master] = big.NewInt(0)
	} else {
		m.increases[master] = new(big.Int).Add(existing, reward)
	}
	return m.increaseRewardErr
}

func TestCalculateRewards(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})
	st.SetStorage(thor.BytesToAddress([]byte("par")), thor.KeyCurveFactor, thor.BytesToBytes32(thor.InitialCurveFactor.Bytes()))

	p := params.New(thor.BytesToAddress([]byte("par")), st)
	eng := New(thor.BytesToAddress([]byte("eng")), st, 1, p, nil)

	mockStaker := &mockStaker{
		lockedVET:         25,
		hasDelegations:    false,
		increaseRewardErr: nil,
	}

	reward, err := eng.CalculateRewards(mockStaker)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(121765601217656012), reward)
}

func TestDistributeRewards(t *testing.T) {
	st := state.New(muxdb.NewMem(), trie.Root{})

	beneficiary := thor.BytesToAddress([]byte("beneficiary"))
	signer := thor.BytesToAddress([]byte("signer"))
	stargateAddr := thor.BytesToAddress([]byte("stargate"))
	energyAddr := thor.BytesToAddress([]byte("eng"))

	paramsAddr := thor.BytesToAddress([]byte("par"))
	p := params.New(paramsAddr, st)

	st.SetStorage(paramsAddr, thor.KeyValidatorRewardPercentage, thor.BytesToBytes32(big.NewInt(int64(thor.InitialValidatorRewardPercentage)).Bytes()))
	st.SetStorage(paramsAddr, thor.KeyDelegatorContractAddress, thor.BytesToBytes32(stargateAddr.Bytes()))

	eng := New(thor.BytesToAddress([]byte("eng")), st, 1, p, nil)

	stake := uint64(25)
	expectedReward := big.NewInt(121765601217656012)

	expectedBeneficiaryReward := big.NewInt(0).Mul(expectedReward, big.NewInt(3))
	expectedBeneficiaryReward = big.NewInt(0).Div(expectedBeneficiaryReward, big.NewInt(10))

	tests := []struct {
		name                      string
		hasDelegations            bool
		expectedErr               error
		expectedBeneficiaryReward *big.Int
		expectedDelegatorReward   *big.Int
		expectedIssued            *big.Int
	}{
		{
			name:                      "No delegations - full reward to beneficiary",
			hasDelegations:            false,
			expectedErr:               nil,
			expectedBeneficiaryReward: expectedReward,
			expectedDelegatorReward:   big.NewInt(0),
			expectedIssued:            expectedReward,
		},
		{
			name:                      "With delegations - split reward",
			hasDelegations:            true,
			expectedErr:               nil,
			expectedBeneficiaryReward: expectedBeneficiaryReward,
			expectedDelegatorReward:   new(big.Int).Sub(expectedReward, expectedBeneficiaryReward),
			expectedIssued:            expectedReward,
		},
		{
			name:                      "With delegations - IncreaseDelegatorsReward fails",
			hasDelegations:            true,
			expectedErr:               errors.New("increase reward failed"),
			expectedBeneficiaryReward: big.NewInt(0),
			expectedDelegatorReward:   big.NewInt(0),
			expectedIssued:            big.NewInt(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state for each test
			st.SetEnergy(beneficiary, big.NewInt(0), 1)
			st.SetEnergy(stargateAddr, big.NewInt(0), 1)
			st.SetStorage(energyAddr, issuedKey, thor.Bytes32{})

			mockStaker := &mockStaker{
				lockedVET:         stake,
				hasDelegations:    tt.hasDelegations,
				increaseRewardErr: tt.expectedErr,
			}

			err := eng.DistributeRewards(beneficiary, signer, mockStaker, 10)
			assert.Equal(t, tt.expectedErr, err)

			beneficiaryEnergy, err := eng.Get(beneficiary)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedBeneficiaryReward, beneficiaryEnergy)

			if tt.hasDelegations {
				stargateEnergy, err := eng.Get(stargateAddr)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedDelegatorReward, stargateEnergy)
			}

			issued, err := eng.getIssued()
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedIssued, issued)
		})
	}
}

func TestDistributeRewards_MaxRewardsPercentage(t *testing.T) {
	// setting rewards over 100 should be capped at 100%
	// setting above 100 should not decrease the delegators rewards
	rewardPercent := big.NewInt(200)

	st := state.New(muxdb.NewMem(), trie.Root{})

	beneficiary := thor.BytesToAddress([]byte("beneficiary"))
	signer := thor.BytesToAddress([]byte("signer"))
	stargateAddr := thor.BytesToAddress([]byte("stargate"))
	energyAddr := thor.BytesToAddress([]byte("eng"))

	paramsAddr := thor.BytesToAddress([]byte("par"))
	p := params.New(paramsAddr, st)

	require.NoError(t, p.Set(thor.KeyValidatorRewardPercentage, rewardPercent))
	require.NoError(t, p.Set(thor.KeyDelegatorContractAddress, new(big.Int).SetBytes(stargateAddr.Bytes())))

	staker := &mockStaker{
		lockedVET:         10000,
		lockedWeight:      22000,
		hasDelegations:    true,
		increaseRewardErr: nil,
	}

	energy := New(energyAddr, st, 100, p)
	issuedBefore, err := energy.getIssued()
	require.NoError(t, err)
	require.NoError(t, energy.DistributeRewards(beneficiary, signer, staker, 10))

	// verify beneficiary received 100%, not 200%
	beneficiaryEnergy, err := energy.Get(beneficiary)
	require.NoError(t, err)

	rewards, err := energy.CalculateRewards(staker)
	require.NoError(t, err)
	assert.Equal(t, rewards, beneficiaryEnergy)

	// verify delegators received 0%
	stargateEnergy, err := energy.Get(stargateAddr)
	require.NoError(t, err)
	assert.Equal(t, big.NewInt(0), stargateEnergy)
	_, delegatorReceived := staker.increases[stargateAddr]
	assert.False(t, delegatorReceived)

	// verify issued amount
	issuedAfter, err := energy.getIssued()
	require.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Add(issuedBefore, rewards), issuedAfter)
}

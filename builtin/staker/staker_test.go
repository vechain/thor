package staker

import (
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
)

type TestFunc func(t *testing.T)

type TestSequence struct {
	staker *Staker

	funcs []TestFunc
	mu    sync.Mutex
}

func NewSequence(staker *Staker) *TestSequence {
	return &TestSequence{funcs: make([]TestFunc, 0), staker: staker}
}

func (st *TestSequence) AddFunc(f TestFunc) *TestSequence {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.funcs = append(st.funcs, f)
	return st
}

func (st *TestSequence) AddValidator(
	endorsor, master thor.Address,
	period uint32,
	stake *big.Int,
) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		err := st.staker.AddValidator(endorsor, master, period, stake)
		if err != nil {
			t.Fatalf("failed to add validator %s: %v", master.String(), err)
		}
		t.Logf("added validator %s", master.String())
	})
}

func (st *TestSequence) AddDelegation(
	master thor.Address,
	amount *big.Int,
	multiplier uint8,
	id *thor.Bytes32,
) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		delegationID, err := st.staker.AddDelegation(master, amount, multiplier)
		if err != nil {
			t.Fatalf("failed to add delegation for validator %s: %v", master.String(), err)
		}
		t.Logf("added delegation %s for validator %s", delegationID.String(), master.String())
		*id = delegationID
	})
}

func (st *TestSequence) ActivateNext(block uint32) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		mbp, err := st.staker.params.Get(thor.KeyMaxBlockProposers)
		if err != nil {
			t.Fatalf("failed to get max block proposers: %v", err)
		}
		addr, err := st.staker.ActivateNextValidator(block, mbp)
		if err != nil {
			t.Fatalf("failed to activate next validator: %v", err)
		}
		t.Logf("activated next validator: %s", addr.String())
	})
}

func (st *TestSequence) SignalExit(endorsor, master thor.Address) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		err := st.staker.SignalExit(endorsor, master)
		if err != nil {
			t.Fatalf("failed to signal exit for validator %s: %v", master, err)
		}
		t.Logf("exit signaled for validator %s", master.String())
	})
}

func (st *TestSequence) Withdraw(endorsor, master thor.Address, block uint32, withdrawable **big.Int) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		amount, err := st.staker.WithdrawStake(endorsor, master, block)
		if err != nil {
			t.Fatalf("failed to withdraw from validator %s: %v", master, err)
		}
		t.Logf("withdrawn %s from validator %s", amount.String(), master.String())
		*withdrawable = amount
	})
}

func (st *TestSequence) Housekeep(block uint32) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		_, _, err := st.staker.Housekeep(block)
		if err != nil {
			t.Fatalf("failed to housekeep at block %d: %v", block, err)
		}
		t.Logf("housekeeping completed at block %d", block)
	})
}

func (st *TestSequence) Transition(block uint32) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		transitioned, err := st.staker.Transition(block)
		if err != nil {
			t.Fatalf("failed to transition at block %d: %v", block, err)
		}
		t.Logf("transitioned at block %d: %v", block, transitioned)
	})
}

func (st *TestSequence) IncreaseStake(
	addr thor.Address,
	endorsor thor.Address,
	amount *big.Int,
) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		err := st.staker.IncreaseStake(addr, endorsor, amount)
		if err != nil {
			t.Fatalf("failed to increase stake for validator %s: %v", addr, err)
		}
		t.Logf("increased stake for validator %s by %s", addr.String(), amount.String())
	})
}

func (st *TestSequence) DecreaseStake(
	addr thor.Address,
	endorsor thor.Address,
	amount *big.Int,
) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		err := st.staker.DecreaseStake(addr, endorsor, amount)
		if err != nil {
			t.Fatalf("failed to decrease stake for validator %s: %v", addr, err)
		}
		t.Logf("decreased stake for validator %s by %s", addr.String(), amount.String())
	})
}

func (st *TestSequence) AssertLockedVET(expectedLocked, expectedWeight *big.Int) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		locked, weight, err := st.staker.LockedVET()
		if err != nil {
			t.Fatalf("failed to get locked VET: %v", err)
		}
		if expectedLocked != nil {
			assert.Equal(t, expectedLocked, locked, "locked VET mismatch")
		}
		if expectedWeight != nil {
			assert.Equal(t, expectedWeight, weight, "locked weight mismatch")
		}
		t.Logf("locked VET: %s, weight: %s", locked.String(), weight.String())
	})
}

func (st *TestSequence) AssertQueuedStake(expectedQueued, expectedWeight *big.Int) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		queued, weight, err := st.staker.QueuedStake()
		if err != nil {
			t.Fatalf("failed to get queued stake: %v", err)
		}
		if expectedQueued != nil {
			assert.Equal(t, expectedQueued, queued, "queued stake mismatch")
		}
		if expectedWeight != nil {
			assert.Equal(t, expectedWeight, weight, "queued weight mismatch")
		}
		t.Logf("queued stake: %s, weight: %s", queued.String(), weight.String())
	})
}

func (st *TestSequence) AssertFirstQueued(expectedAddr *thor.Address) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		firstQueued, err := st.staker.FirstQueued()
		if err != nil {
			t.Fatalf("failed to get first queued: %v", err)
		}
		if expectedAddr == nil {
			assert.Nil(t, firstQueued, "expected no queued validators")
		} else {
			assert.NotNil(t, firstQueued, "expected queued validator")
			assert.Equal(t, *expectedAddr, *firstQueued, "first queued validator mismatch")
		}
		if firstQueued != nil {
			t.Logf("first queued validator: %s", firstQueued.String())
		} else {
			t.Logf("no queued validators")
		}
	})
}

func (st *TestSequence) AssertFirstActive(expectedAddr *thor.Address) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		firstActive, err := st.staker.FirstActive()
		if err != nil {
			t.Fatalf("failed to get first active: %v", err)
		}
		if expectedAddr == nil {
			assert.Nil(t, firstActive, "expected no active validators")
		} else {
			assert.NotNil(t, firstActive, "expected active validator")
			assert.Equal(t, *expectedAddr, *firstActive, "first active validator mismatch")
		}
		if firstActive != nil {
			t.Logf("first active validator: %s", firstActive.String())
		} else {
			t.Logf("no active validators")
		}
	})
}

func (st *TestSequence) Run(t *testing.T) {
	st.mu.Lock()
	defer st.mu.Unlock()

	for _, f := range st.funcs {
		f(t)
	}

	t.Logf("All test functions executed successfully")
}

type ValidatorAssertions struct {
	staker *Staker
	addr   thor.Address

	status             *Status
	weight             *big.Int
	stake              *big.Int
	pendingLocked      *big.Int
	cooldownVET        *big.Int
	withdrawableVET    *big.Int
	exitingVET         *big.Int
	period             *uint32
	nextPeriodDecrease *big.Int
	isEmpty            *bool
}

func AssertValidator(staker *Staker, addr thor.Address) *ValidatorAssertions {
	return &ValidatorAssertions{staker: staker, addr: addr}
}

func (va *ValidatorAssertions) Status(expected Status) *ValidatorAssertions {
	va.status = &expected
	return va
}

func (va *ValidatorAssertions) Weight(expected *big.Int) *ValidatorAssertions {
	va.weight = expected
	return va
}

func (va *ValidatorAssertions) Stake(expected *big.Int) *ValidatorAssertions {
	va.stake = expected
	return va
}

func (va *ValidatorAssertions) PendingLocked(expected *big.Int) *ValidatorAssertions {
	va.pendingLocked = expected
	return va
}

func (va *ValidatorAssertions) CooldownVET(expected *big.Int) *ValidatorAssertions {
	va.cooldownVET = expected
	return va
}

func (va *ValidatorAssertions) ExitingVET(expected *big.Int) *ValidatorAssertions {
	// Alias for WithdrawableVET, used for backward compatibility
	va.exitingVET = expected
	return va
}

func (va *ValidatorAssertions) WithdrawableVET(expected *big.Int) *ValidatorAssertions {
	va.withdrawableVET = expected
	return va
}

func (va *ValidatorAssertions) Period(expected uint32) *ValidatorAssertions {
	va.period = &expected
	return va
}

func (va *ValidatorAssertions) NextPeriodDecrease(expected *big.Int) *ValidatorAssertions {
	va.nextPeriodDecrease = expected
	return va
}

func (va *ValidatorAssertions) IsEmpty(expected bool) *ValidatorAssertions {
	va.isEmpty = &expected
	return va
}

func (va *ValidatorAssertions) Assert(t *testing.T) {
	validator, err := va.staker.Get(va.addr)
	assert.NoError(t, err, "failed to get validator %s", va.addr.String())

	if va.isEmpty != nil {
		assert.Equal(t, *va.isEmpty, validator.IsEmpty(), "validator %s empty state mismatch", va.addr.String())
		if *va.isEmpty {
			return // If we expect it to be empty, don't check other fields
		}
	}

	if va.status != nil {
		assert.Equal(t, *va.status, validator.Status, "validator %s status mismatch", va.addr.String())
	}

	if va.weight != nil {
		assert.Equal(t, va.weight, validator.Weight, "validator %s weight mismatch", va.addr.String())
	}

	if va.stake != nil {
		assert.Equal(t, va.stake, validator.LockedVET, "validator %s stake mismatch", va.addr.String())
	}

	if va.pendingLocked != nil {
		assert.Equal(t, va.pendingLocked, validator.PendingLocked, "validator %s pending locked mismatch", va.addr.String())
	}

	if va.cooldownVET != nil {
		assert.Equal(t, va.cooldownVET, validator.CooldownVET, "validator %s cooldown VET mismatch", va.addr.String())
	}

	if va.withdrawableVET != nil {
		assert.Equal(t, va.withdrawableVET, validator.WithdrawableVET, "validator %s withdrawable VET mismatch", va.addr.String())
	}

	if va.exitingVET != nil {
		assert.Equal(t, va.exitingVET, validator.NextPeriodDecrease, "validator %s exiting VET mismatch", va.addr.String())
	}

	if va.period != nil {
		assert.Equal(t, *va.period, validator.Period, "validator %s period mismatch", va.addr.String())
	}

	if va.nextPeriodDecrease != nil {
		assert.Equal(t, va.nextPeriodDecrease, validator.NextPeriodDecrease, "validator %s next period decrease mismatch", va.addr.String())
	}
}

type AggregationAssertions struct {
	staker       *Staker
	validationID thor.Address

	pendingVET    *big.Int
	pendingWeight *big.Int

	lockedVET    *big.Int
	lockedWeight *big.Int

	exitingVET    *big.Int
	exitingWeight *big.Int

	withdrawableVET *big.Int
}

func AssertAggregation(staker *Staker, validationID thor.Address) *AggregationAssertions {
	return &AggregationAssertions{staker: staker, validationID: validationID}
}

func (aa *AggregationAssertions) PendingVET(expected *big.Int) *AggregationAssertions {
	aa.pendingVET = expected
	return aa
}

func (aa *AggregationAssertions) PendingWeight(expected *big.Int) *AggregationAssertions {
	aa.pendingWeight = expected
	return aa
}

func (aa *AggregationAssertions) LockedVET(expected *big.Int) *AggregationAssertions {
	aa.lockedVET = expected
	return aa
}

func (aa *AggregationAssertions) LockedWeight(expected *big.Int) *AggregationAssertions {
	aa.lockedWeight = expected
	return aa
}

func (aa *AggregationAssertions) ExitingVET(expected *big.Int) *AggregationAssertions {
	aa.exitingVET = expected
	return aa
}

func (aa *AggregationAssertions) ExitingWeight(expected *big.Int) *AggregationAssertions {
	aa.exitingWeight = expected
	return aa
}

func (aa *AggregationAssertions) WithdrawableVET(expected *big.Int) *AggregationAssertions {
	aa.withdrawableVET = expected
	return aa
}

func (aa *AggregationAssertions) Assert(t *testing.T) {
	agg, err := aa.staker.aggregationService.GetAggregation(aa.validationID)
	assert.NoError(t, err, "failed to get aggregation for validator %s", aa.validationID.String())

	if agg.IsEmpty() {
		t.Fatalf("aggregation for validator %s is empty, expected non-empty aggregation", aa.validationID.String())
	}

	if aa.pendingVET != nil {
		assert.Equal(t, aa.pendingVET, agg.PendingVET, "pending VET mismatch for validator %s", aa.validationID.String())
	}

	if aa.pendingWeight != nil {
		assert.Equal(t, aa.pendingWeight, agg.PendingWeight, "pending weight mismatch for validator %s", aa.validationID.String())
	}

	if aa.lockedVET != nil {
		assert.Equal(t, aa.lockedVET, agg.LockedVET, "locked VET mismatch for validator %s", aa.validationID.String())
	}

	if aa.lockedWeight != nil {
		assert.Equal(t, aa.lockedWeight, agg.LockedWeight, "locked weight mismatch for validator %s", aa.validationID.String())
	}

	if aa.exitingVET != nil {
		assert.Equal(t, aa.exitingVET, agg.ExitingVET, "exiting VET mismatch for validator %s", aa.validationID.String())
	}

	if aa.exitingWeight != nil {
		assert.Equal(t, aa.exitingWeight, agg.ExitingWeight, "exiting weight mismatch for validator %s", aa.validationID.String())
	}

	if aa.withdrawableVET != nil {
		assert.Equal(t, aa.withdrawableVET, agg.WithdrawableVET, "withdrawable VET mismatch for validator %s", aa.validationID.String())
	}
}

type DelegationAssertions struct {
	staker       *Staker
	delegationID thor.Bytes32

	validationID   *thor.Address
	stake          *big.Int
	multiplier     *uint8
	firstIteration *uint32
	lastIteration  *uint32
	weight         *big.Int
	locked         *bool
	started        *bool
	finished       *bool
}

func AssertDelegation(staker *Staker, delegationID thor.Bytes32) *DelegationAssertions {
	return &DelegationAssertions{staker: staker, delegationID: delegationID}
}

func (da *DelegationAssertions) ValidationID(expected thor.Address) *DelegationAssertions {
	da.validationID = &expected
	return da
}

func (da *DelegationAssertions) Stake(expected *big.Int) *DelegationAssertions {
	da.stake = expected
	return da
}

func (da *DelegationAssertions) Weight(expected *big.Int) *DelegationAssertions {
	da.weight = expected
	return da
}

func (da *DelegationAssertions) Multiplier(expected uint8) *DelegationAssertions {
	da.multiplier = &expected
	return da
}

func (da *DelegationAssertions) FirstIteration(expected uint32) *DelegationAssertions {
	da.firstIteration = &expected
	return da
}

func (da *DelegationAssertions) LastIteration(expected uint32) *DelegationAssertions {
	da.lastIteration = &expected
	return da
}

func (da *DelegationAssertions) IsLocked(expected bool) *DelegationAssertions {
	da.locked = &expected
	return da
}

func (da *DelegationAssertions) IsStarted(expected bool) *DelegationAssertions {
	da.started = &expected
	return da
}

func (da *DelegationAssertions) IsFinished(expected bool) *DelegationAssertions {
	da.finished = &expected
	return da
}

func (da *DelegationAssertions) Assert(t *testing.T) {
	delegation, validator, err := da.staker.GetDelegation(da.delegationID)
	assert.NoError(t, err, "failed to get delegation %s", da.delegationID.String())

	if delegation.IsEmpty() {
		t.Fatalf("delegation %s is empty, expected non-empty delegation", da.delegationID.String())
	}

	if da.weight != nil {
		assert.Equal(t, da.weight, delegation.CalcWeight(), "delegation %s weight mismatch", da.delegationID.String())
	}

	if da.locked != nil {
		if *da.locked {
			assert.True(t, delegation.Started(validator), "delegation %s locked state mismatch", da.delegationID.String())
			assert.False(t, delegation.Ended(validator), "delegation %s ended state mismatch", da.delegationID.String())
		} else {
			assert.False(t, delegation.Started(validator), "delegation %s locked state mismatch", da.delegationID.String())
			assert.False(t, delegation.Ended(validator), "delegation %s ended state mismatch", da.delegationID.String())
		}
	}

	if da.started != nil {
		assert.Equal(t, *da.started, delegation.Started(validator), "delegation %s started state mismatch", da.delegationID.String())
	}

	if da.finished != nil {
		assert.Equal(t, *da.finished, delegation.Ended(validator), "delegation % s finished state mismatch", da.delegationID.String())
	}

	if da.validationID != nil {
		assert.Equal(t, *da.validationID, delegation.ValidationID, "delegation %s validation ID mismatch", da.delegationID.String())
	}

	if da.stake != nil {
		assert.Equal(t, da.stake, delegation.Stake, "delegation %s stake mismatch", da.delegationID.String())
	}

	if da.multiplier != nil {
		assert.Equal(t, *da.multiplier, delegation.Multiplier, "delegation %s multiplier mismatch", da.delegationID.String())
	}

	if da.firstIteration != nil {
		assert.Equal(t, *da.firstIteration, delegation.FirstIteration, "delegation %s first iteration mismatch", da.delegationID.String())
	}

	assert.Equal(t, da.lastIteration, delegation.LastIteration, "delegation %s last iteration mismatch", da.delegationID.String())
}

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
	addr thor.Address,
	stake *big.Int,
	period uint32,
) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		err := st.staker.AddValidator(addr, addr, period, stake)
		if err != nil {
			t.Fatalf("failed to add validator %s: %v", addr, err)
		}
		t.Logf("added validator %s", addr.String())
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

func (st *TestSequence) SignalExit(addr thor.Address) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		err := st.staker.SignalExit(addr, addr)
		if err != nil {
			t.Fatalf("failed to signal exit for validator %s: %v", addr, err)
		}
		t.Logf("exit signaled for validator %s", addr.String())
	})
}

func (st *TestSequence) Withdraw(addr thor.Address, block uint32) *TestSequence {
	return st.AddFunc(func(t *testing.T) {
		amount, err := st.staker.WithdrawStake(addr, addr, block)
		if err != nil {
			t.Fatalf("failed to withdraw from validator %s: %v", addr, err)
		}
		t.Logf("withdrawn %s from validator %s", amount.String(), addr.String())
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

	status *Status
	weight *big.Int
	stake  *big.Int
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

func (va *ValidatorAssertions) Assert(t *testing.T) {
	validator, err := va.staker.Get(va.addr)
	assert.NoError(t, err, "failed to get validator %s", va.addr.String())

	if va.status != nil {
		assert.Equal(t, *va.status, validator.Status, "validator %s status mismatch", va.addr.String())
	}

	if va.weight != nil {
		assert.Equal(t, va.weight, validator.Weight, "validator %s weight mismatch", va.addr.String())
	}

	if va.stake != nil {
		assert.Equal(t, va.stake, validator.LockedVET, "validator %s stake mismatch", va.addr.String())
	}
}

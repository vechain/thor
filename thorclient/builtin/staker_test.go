// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package builtin

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/bind"
	"github.com/vechain/thor/v2/tx"
)

func DebugRevert(t *testing.T, receipt *api.Receipt, sender *bind.MethodBuilder) {
	if receipt == nil {
		assert.Fail(t, "receipt is nil")
		return
	}
	if receipt.Reverted {
		_, err := sender.Call().
			AtRevision(receipt.Meta.BlockID.String()).
			Caller(&receipt.Meta.TxOrigin).
			Execute()
		if err != nil {
			assert.Fail(t, "transaction reverted", err)
		} else {
			assert.Fail(t, "transaction reverted for unknown reason")
		}
	}
}

func TestStaker(t *testing.T) {
	minStakingPeriod := uint32(360 * 24 * 7) // 7 days in blocks

	node, client := newTestNode(t, false)
	defer node.Stop()

	// builtins
	staker, err := NewStaker(client)
	assert.NoError(t, err)
	authority, err := NewAuthority(client)
	assert.NoError(t, err)
	params, err := NewParams(client)
	assert.NoError(t, err)

	// set authorities - required for initial staker setup
	var authorityTxs []*bind.SendBuilder
	executor := bind.NewSigner(genesis.DevAccounts()[0].PrivateKey)
	stargate := bind.NewSigner(genesis.DevAccounts()[0].PrivateKey)
	for _, acc := range genesis.DevAccounts()[1:] {
		sender := authority.Add(acc.Address, acc.Address, datagen.RandomHash()).Send().WithSigner(executor).WithOptions(txOpts())
		authorityTxs = append(authorityTxs, sender)
	}
	for _, tx := range authorityTxs {
		if _, err := tx.Submit(); err != nil {
			t.Fatal(err)
		}
	}

	// set max block proposers
	if _, _, err := params.Set(thor.KeyMaxBlockProposers, big.NewInt(3)).
		Send().
		WithSigner(executor).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t)); err != nil {
		t.Fatal(err)
	}
	// set stargate address
	if _, _, err := params.Set(thor.KeyStargateContractAddress, new(big.Int).SetBytes(stargate.Address().Bytes())).
		Send().
		WithSigner(executor).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t)); err != nil {
		t.Fatal(err)
	}

	// add validators - trigger PoS activation
	minStake := MinStake()
	var validatorTxs []*tx.Transaction
	for _, acc := range genesis.DevAccounts()[0:2] {
		addValidatorTx, err := staker.AddValidation(acc.Address, minStake, minStakingPeriod).
			Send().
			WithSigner(bind.NewSigner(acc.PrivateKey)).
			WithOptions(txOpts()).
			Submit()
		assert.NoError(t, err)
		validatorTxs = append(validatorTxs, addValidatorTx)
	}
	for i, trx := range validatorTxs {
		assert.NoError(t, test.Retry(func() error {
			id := trx.ID()
			receipt, err := client.TransactionReceipt(&id)
			if err != nil {
				return err
			}
			DebugRevert(t, receipt, staker.AddValidation(genesis.DevAccounts()[i].Address, minStake, minStakingPeriod))
			return nil
		}, 100*time.Millisecond, 10*time.Second))
	}

	// pack a new block
	assert.NoError(t, node.Chain().MintBlock(genesis.DevAccounts()[0]))

	// TotalStake
	totalStake, totalWeight, err := staker.TotalStake()
	assert.NoError(t, err)
	assert.Equal(t, 1, totalWeight.Sign())
	assert.Equal(t, new(big.Int).Mul(minStake, big.NewInt(2)), totalStake)

	// GetStake
	_, firstID, err := staker.FirstActive()
	assert.NoError(t, err)
	getStakeRes, err := staker.GetValidatorStake(firstID)
	assert.NoError(t, err)
	assert.False(t, getStakeRes.Address.IsZero())
	assert.False(t, getStakeRes.Endorsor.IsZero())
	assert.Equal(t, getStakeRes.Stake, minStake)
	assert.Equal(t, getStakeRes.Weight, big.NewInt(0).Mul(minStake, big.NewInt(2)))
	assert.Equal(t, getStakeRes.QueuedStake.String(), big.NewInt(0).String())

	// GetStatus
	getStatusRes, err := staker.GetValidatorStatus(firstID)
	assert.NoError(t, err)
	assert.Equal(t, firstID, getStatusRes.Address)
	assert.Equal(t, StakerStatusActive, getStatusRes.Status)
	assert.True(t, getStatusRes.Online)
	assert.True(t, getStakeRes.Exists(*getStatusRes))

	// GetPeriodDetails
	getPeriodDetailsRes, err := staker.GetValidatorPeriodDetails(firstID)
	assert.NoError(t, err)
	assert.Equal(t, firstID, getPeriodDetailsRes.Address)
	assert.Equal(t, minStakingPeriod, getPeriodDetailsRes.Period)
	assert.Equal(t, uint32(15), getPeriodDetailsRes.StartBlock)
	assert.Equal(t, uint32(math.MaxUint32), getPeriodDetailsRes.ExitBlock)
	assert.Equal(t, uint32(0), getPeriodDetailsRes.CompletedPeriods)

	// FirstActive
	firstActive, firstID, err := staker.FirstActive()
	assert.NoError(t, err)
	assert.False(t, firstID.IsZero())
	assert.True(t, firstActive.Exists(*getStatusRes))
	assert.Equal(t, minStake, firstActive.Stake)
	assert.Equal(t, big.NewInt(0).Mul(minStake, big.NewInt(2)), firstActive.Weight)
	assert.False(t, firstActive.Endorsor.IsZero())

	// Next
	next, id, err := staker.Next(firstID)
	assert.NoError(t, err)
	assert.False(t, id.IsZero())
	nextStatus, err := staker.GetValidatorStatus(next.Address)
	assert.True(t, next.Exists(*nextStatus))
	assert.NoError(t, err)
	assert.Equal(t, StakerStatusActive, nextStatus.Status)
	assert.Equal(t, minStake, next.Stake)
	assert.Equal(t, big.NewInt(0).Mul(minStake, big.NewInt(2)), next.Weight)
	assert.False(t, next.Endorsor.IsZero())

	var (
		validator    = genesis.DevAccounts()[9]
		validatorKey = bind.NewSigner(validator.PrivateKey)
	)

	// AddValidator
	receipt, _, err := staker.AddValidation(validator.Address, minStake, minStakingPeriod).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	assert.NoError(t, err)
	assert.False(t, receipt.Reverted)

	queuedEvents, err := staker.FilterValidatorQueued(newRange(receipt), nil, logdb.ASC)
	assert.NoError(t, err)
	assert.Len(t, queuedEvents, 1)
	assert.Equal(t, validator.Address, queuedEvents[0].Endorsor)
	assert.Equal(t, minStake, queuedEvents[0].Stake)
	queuedID := queuedEvents[0].Node

	// FirstQueued
	firstQueued, id, err := staker.FirstQueued()
	assert.NoError(t, err)
	assert.False(t, id.IsZero())
	firstQueuedStatus, err := staker.GetValidatorStatus(firstQueued.Address)
	assert.NoError(t, err)
	assert.True(t, firstQueued.Exists(*firstQueuedStatus))
	assert.Equal(t, 0, firstQueued.Stake.Sign())
	assert.Equal(t, StakerStatusQueued, firstQueuedStatus.Status)
	assert.False(t, firstQueued.Endorsor.IsZero())

	// TotalQueued
	queuedStake, queuedWeight, err := staker.QueuedStake()
	assert.NoError(t, err)
	stake := big.NewInt(0).Mul(big.NewInt(1e18), big.NewInt(25))
	stake = big.NewInt(0).Mul(stake, big.NewInt(1e6))
	assert.Equal(t, stake, queuedStake)
	assert.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)

	// IncreaseStake
	receipt, _, err = staker.IncreaseStake(queuedID, minStake).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	assert.NoError(t, err)

	increaseEvents, err := staker.FilterStakeIncreased(newRange(receipt), nil, logdb.ASC)
	assert.NoError(t, err)
	assert.Len(t, increaseEvents, 1)
	assert.Equal(t, validator.Address, increaseEvents[0].Validator)
	assert.Equal(t, minStake, increaseEvents[0].Added)

	// DecreaseStake
	receipt, _, err = staker.DecreaseStake(queuedID, minStake).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	assert.NoError(t, err)

	decreaseEvents, err := staker.FilterStakeDecreased(newRange(receipt), nil, logdb.ASC)
	assert.NoError(t, err)
	assert.Len(t, decreaseEvents, 1)
	assert.Equal(t, queuedID, decreaseEvents[0].Validator)
	assert.Equal(t, minStake, decreaseEvents[0].Removed)

	// SignalExit
	receipt, _, err = staker.SignalExit(queuedID).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	assert.NoError(t, err)

	// No events for signal exit when state is queued
	autoRenewEvents, err := staker.FilterValidationSignaledExit(newRange(receipt), nil, logdb.ASC)
	assert.NoError(t, err)
	assert.Len(t, autoRenewEvents, 0)

	// AddDelegation
	method := staker.AddDelegation(queuedID, minStake, 100)
	receipt, _, err = method.
		Send().
		WithSigner(stargate).
		WithOptions(txOpts()).
		SubmitAndConfirm(txContext(t))
	assert.NoError(t, err)
	DebugRevert(t, receipt, method)

	delegationEvents, err := staker.FilterDelegationAdded(newRange(receipt), nil, logdb.ASC)
	assert.NoError(t, err)
	assert.Len(t, delegationEvents, 1)
	delegationID := delegationEvents[0].DelegationID

	// GetDelegationStake
	delegationStake, err := staker.GetDelegationStake(delegationID)
	assert.NoError(t, err)
	assert.Equal(t, minStake, delegationStake.Stake)
	assert.Equal(t, uint8(100), delegationStake.Multiplier)
	assert.Equal(t, queuedID, delegationStake.Validator)

	// GetDelegationPeriodDetails
	delegationPeriodDetails, err := staker.GetDelegationPeriodDetails(delegationID)
	assert.NoError(t, err)
	assert.Equal(t, uint32(1), delegationPeriodDetails.StartPeriod)
	assert.Equal(t, uint32(math.MaxUint32), delegationPeriodDetails.EndPeriod)
	assert.False(t, delegationPeriodDetails.Locked)

	// GetValidatorsTotals
	validationTotals, err := staker.GetValidationTotals(firstID)
	assert.NoError(t, err)

	assert.Equal(t, minStake, validationTotals.TotalLockedStake)
	assert.Equal(t, big.NewInt(0).Mul(minStake, big.NewInt(2)), validationTotals.TotalLockedWeight)
	assert.Equal(t, big.NewInt(0).String(), validationTotals.TotalQueuedStake.String())
	assert.Equal(t, big.NewInt(0).String(), validationTotals.TotalQueuedWeight.String())
	assert.Equal(t, big.NewInt(0).String(), validationTotals.TotalExitingStake.String())
	assert.Equal(t, big.NewInt(0).String(), validationTotals.TotalExitingWeight.String())

	// GetValidatorsNum
	active, queued, err := staker.GetValidatorsNum()
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(2), active)
	assert.Equal(t, big.NewInt(1), queued)

	// UpdateDelegationAutoRenew - Enable AutoRenew
	receipt, _, err = staker.SignalDelegationExit(delegationID).
		Send().
		WithSigner(stargate).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	assert.NoError(t, err)
	assert.True(t, receipt.Reverted) // should revert since it hasn't started

	// Withdraw
	receipt, _, err = staker.WithdrawStake(queuedID).Send().WithSigner(validatorKey).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	assert.NoError(t, err)
	assert.False(t, receipt.Reverted)

	withdrawEvents, err := staker.FilterValidationWithdrawn(newRange(receipt), nil, logdb.ASC)
	assert.NoError(t, err)
	assert.Len(t, withdrawEvents, 1)

	getStatusRes, err = staker.GetValidatorStatus(queuedID)
	assert.NoError(t, err)
	assert.Equal(t, StakerStatusExited, getStatusRes.Status)

	// WithdrawDelegation
	receipt, _, err = staker.WithdrawDelegation(delegationID).Send().WithSigner(stargate).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	assert.NoError(t, err)
	DebugRevert(t, receipt, staker.WithdrawDelegation(delegationID))

	withdrawDelegationEvents, err := staker.FilterDelegationWithdrawn(newRange(receipt), nil, logdb.ASC)
	assert.NoError(t, err)
	assert.Len(t, withdrawDelegationEvents, 1)

	// GetDelegation after withdrawal
	delegationStake, err = staker.GetDelegationStake(delegationID)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0).Cmp(delegationStake.Stake), 0)

	// GetDelegatorsRewards
	rewards, err := staker.GetDelegatorsRewards(validator.Address, 1)
	assert.NoError(t, err)
	assert.Equal(t, 0, big.NewInt(0).Cmp(rewards))

	// Issuance
	issuance, err := staker.Issuance("best")
	assert.NoError(t, err)
	assert.Equal(t, 1, issuance.Sign())

	best := node.Chain().Repo().BestBlockSummary()
	state := node.Chain().Stater().NewState(best.Root())
	energy := builtin.Energy.Native(state, best.Header.Timestamp())
	stakerNative := builtin.Staker.Native(state)

	rewards, err = energy.CalculateRewards(stakerNative)
	assert.NoError(t, err)
	assert.Equal(t, rewards, issuance)
}

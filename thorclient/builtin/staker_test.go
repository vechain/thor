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

	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/bind"
	"github.com/vechain/thor/v2/tx"
)

func TestStaker(t *testing.T) {
	minStakingPeriod := uint32(360) * 24 * 7 // 360 days in seconds

	node, client := newTestNode(t, false)
	defer node.Stop()

	// builtins
	staker, err := NewStaker(client)
	require.NoError(t, err)
	authority, err := NewAuthority(client)
	require.NoError(t, err)
	params, err := NewParams(client)
	require.NoError(t, err)

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
			WithOptions(txOpts()).Submit()
		require.NoError(t, err)
		validatorTxs = append(validatorTxs, addValidatorTx)
	}
	for _, trx := range validatorTxs {
		require.NoError(t, test.Retry(func() error {
			id := trx.ID()
			if _, err = client.TransactionReceipt(&id); err != nil {
				return err
			}
			return nil
		}, 100*time.Millisecond, 10*time.Second))
	}

	// pack a new block
	require.NoError(t, node.Chain().MintBlock(genesis.DevAccounts()[0]))

	// TotalStake
	totalStake, totalWeight, err := staker.TotalStake()
	require.NoError(t, err)
	require.Equal(t, 1, totalWeight.Sign())
	require.Equal(t, new(big.Int).Mul(minStake, big.NewInt(2)), totalStake)

	// Get
	_, firstID, err := staker.FirstActive()
	require.NoError(t, err)
	getRes, err := staker.Get(firstID)
	require.NoError(t, err)
	require.False(t, getRes.Address.IsZero())
	require.False(t, getRes.Endorsor.IsZero())
	require.Equal(t, StakerStatusActive, getRes.Status)
	require.Equal(t, getRes.Stake, minStake)
	require.Equal(t, getRes.Weight, big.NewInt(0).Mul(minStake, big.NewInt(2)))

	// FirstActive
	firstActive, firstID, err := staker.FirstActive()
	require.NoError(t, err)
	require.False(t, firstID.IsZero())
	require.True(t, firstActive.Exists())
	require.Equal(t, minStake, firstActive.Stake)
	require.Equal(t, big.NewInt(0).Mul(minStake, big.NewInt(2)), firstActive.Weight)
	require.Equal(t, StakerStatusActive, firstActive.Status)
	require.False(t, firstActive.Endorsor.IsZero())
	require.Greater(t, firstActive.StartBlock, uint32(0))
	require.Equal(t, firstActive.ExitBlock, uint32(math.MaxUint32))

	// LookupNode
	getRes, err = staker.Get(firstActive.Address)
	require.NoError(t, err)
	require.True(t, getRes.Exists())

	// Next
	next, id, err := staker.Next(firstID)
	require.NoError(t, err)
	require.False(t, id.IsZero())
	require.True(t, next.Exists())
	require.Equal(t, StakerStatusActive, next.Status)
	require.Equal(t, minStake, next.Stake)
	require.Equal(t, big.NewInt(0).Mul(minStake, big.NewInt(2)), next.Weight)
	require.False(t, next.Endorsor.IsZero())

	var (
		validator    = genesis.DevAccounts()[9]
		validatorKey = bind.NewSigner(validator.PrivateKey)
	)

	// AddValidator
	receipt, _, err := staker.AddValidation(validator.Address, minStake, minStakingPeriod).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	queuedEvents, err := staker.FilterValidatorQueued(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, queuedEvents, 1)
	require.Equal(t, validator.Address, queuedEvents[0].Endorsor)
	require.Equal(t, minStake, queuedEvents[0].Stake)
	queuedID := queuedEvents[0].Node

	// FirstQueued
	firstQueued, id, err := staker.FirstQueued()
	require.NoError(t, err)
	require.False(t, id.IsZero())
	require.True(t, firstQueued.Exists())
	require.Equal(t, 0, firstQueued.Stake.Sign())
	require.Equal(t, StakerStatusQueued, firstQueued.Status)
	require.False(t, firstQueued.Endorsor.IsZero())

	// TotalQueued
	queuedStake, queuedWeight, err := staker.QueuedStake()
	require.NoError(t, err)
	stake := big.NewInt(0).Mul(big.NewInt(1e18), big.NewInt(25))
	stake = big.NewInt(0).Mul(stake, big.NewInt(1e6))
	require.Equal(t, stake, queuedStake)
	require.Equal(t, big.NewInt(0).Mul(stake, big.NewInt(2)), queuedWeight)

	// IncreaseStake
	receipt, _, err = staker.IncreaseStake(queuedID, minStake).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)

	increaseEvents, err := staker.FilterStakeIncreased(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, increaseEvents, 1)
	require.Equal(t, validator.Address, increaseEvents[0].Validator)
	require.Equal(t, minStake, increaseEvents[0].Added)

	// DecreaseStake
	receipt, _, err = staker.DecreaseStake(queuedID, minStake).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)

	decreaseEvents, err := staker.FilterStakeDecreased(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, decreaseEvents, 1)
	require.Equal(t, queuedID, decreaseEvents[0].Validator)
	require.Equal(t, minStake, decreaseEvents[0].Removed)

	// SignalExit
	receipt, _, err = staker.SignalExit(queuedID).
		Send().
		WithSigner(validatorKey).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)

	// No events for signal exit when state is queued
	autoRenewEvents, err := staker.FilterValidationSignaledExit(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, autoRenewEvents, 0)

	// AddDelegation
	receipt, _, err = staker.AddDelegation(queuedID, minStake, 100).
		Send().
		WithSigner(stargate).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	delegationEvents, err := staker.FilterDelegationAdded(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, delegationEvents, 1)
	delegationID := delegationEvents[0].DelegationID

	// GetDelegation
	delegation, err := staker.GetDelegation(delegationID)
	require.NoError(t, err)
	require.Equal(t, minStake, delegation.Stake)
	require.Equal(t, uint8(100), delegation.Multiplier)
	require.False(t, delegation.Locked)
	require.Equal(t, queuedID, delegation.Validator)

	// GetValidatorsTotals
	validationTotals, err := staker.GetValidationTotals(firstID)
	require.NoError(t, err)

	require.Equal(t, minStake, validationTotals.TotalLockedStake)
	require.Equal(t, big.NewInt(0).Mul(minStake, big.NewInt(2)), validationTotals.TotalLockedWeight)
	require.Equal(t, big.NewInt(0).String(), validationTotals.DelegationsLockedWeight.String())
	require.Equal(t, big.NewInt(0).String(), validationTotals.DelegationsLockedStake.String())

	// GetValidatorsNum
	active, queued, err := staker.GetValidatorsNum()
	require.NoError(t, err)
	require.Equal(t, big.NewInt(2), active)
	require.Equal(t, big.NewInt(1), queued)

	// UpdateDelegationAutoRenew - Enable AutoRenew
	receipt, _, err = staker.SignalDelegationExit(delegationID).
		Send().
		WithSigner(stargate).
		WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.True(t, receipt.Reverted) // should revert since it hasn't started

	// Withdraw
	receipt, _, err = staker.WithdrawStake(queuedID).Send().WithSigner(validatorKey).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	withdrawEvents, err := staker.FilterValidationWithdrawn(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, withdrawEvents, 1)

	getRes, err = staker.Get(queuedID)
	require.NoError(t, err)
	require.Equal(t, StakerStatusExited, getRes.Status)

	// WithdrawDelegation
	receipt, _, err = staker.WithdrawDelegation(delegationID).Send().WithSigner(stargate).WithOptions(txOpts()).SubmitAndConfirm(txContext(t))
	require.NoError(t, err)
	require.False(t, receipt.Reverted)

	withdrawDelegationEvents, err := staker.FilterDelegationWithdrawn(newRange(receipt), nil, logdb.ASC)
	require.NoError(t, err)
	require.Len(t, withdrawDelegationEvents, 1)

	// GetDelegation after withdrawal
	delegation, err = staker.GetDelegation(delegationID)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(0).Cmp(delegation.Stake), 0)

	// GetDelegatorsRewards
	rewards, err := staker.GetDelegatorsRewards(validator.Address, 1)
	require.NoError(t, err)
	require.Equal(t, 0, big.NewInt(0).Cmp(rewards))
}

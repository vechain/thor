// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/vechain/thor/v2/log"

	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// TODO: Do these need to be set in params.sol, or some other dynamic way?
var (
	logger   = log.WithContext("pkg", "staker")
	minStake = big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	maxStake = big.NewInt(0).Mul(big.NewInt(400e6), big.NewInt(1e18))

	// TODO: Enable these once customnet testing is done
	//oneWeekStakingPeriod  = uint32(360) * 24 * 7     // 1 weeks
	//twoWeeksStakingPeriod = oneWeekStakingPeriod * 2 // 2 weeks
	//oneMonthStakingPeriod = uint32(360) * 24 * 30    // 30 days

	cooldownPeriod = uint32(8640)
	epochLength    = uint32(180)
)

// Staker implements native methods of `Staker` contract.
type Staker struct {
	lockedVET   *solidity.Uint256
	queuedVET   *solidity.Uint256
	delegations *delegations
	validations *validations
	storage     *storage
	params      *params.Params
}

// New create a new instance.
func New(addr thor.Address, state *state.State, params *params.Params) *Staker {
	storage := newStorage(addr, state)

	if num, err := solidity.NewUint256(addr, state, thor.BytesToBytes32([]byte("epoch-length"))).Get(); err == nil {
		numUint64 := num.Uint64()
		if numUint64 != 0 {
			epochLength = uint32(numUint64)
		}
	}

	if num, err := solidity.NewUint256(addr, state, thor.BytesToBytes32([]byte("cooldown-period"))).Get(); err == nil {
		numUint64 := num.Uint64()
		if numUint64 != 0 {
			cooldownPeriod = uint32(numUint64)
		}
	}

	return &Staker{
		lockedVET:   solidity.NewUint256(addr, state, slotLockedVET),
		queuedVET:   solidity.NewUint256(addr, state, slotQueuedVET),
		storage:     storage,
		validations: newValidations(storage),
		delegations: newDelegations(storage),
		params:      params,
	}
}

// IsActive checks if the staker contract has become active, i.e. we have transitioned to PoS.
func (s *Staker) IsActive() (bool, error) {
	return s.validations.IsActive()
}

// LeaderGroup lists all registered candidates.
func (s *Staker) LeaderGroup() (map[thor.Bytes32]*Validation, error) {
	return s.validations.LeaderGroup()
}

// LockedVET returns the amount of VET locked by validators and delegators.
func (s *Staker) LockedVET() (*big.Int, error) {
	return s.lockedVET.Get()
}

// QueuedStake returns the amount of VET queued by validators and delegators.
func (s *Staker) QueuedStake() (*big.Int, error) {
	return s.queuedVET.Get()
}

// FirstActive returns validator address of first entry.
func (s *Staker) FirstActive() (thor.Bytes32, error) {
	return s.validations.FirstActive()
}

// FirstQueued returns validator address of first entry.
func (s *Staker) FirstQueued() (thor.Bytes32, error) {
	return s.validations.FirstQueued()
}

// QueuedGroupSize returns the number of validations in the queue
func (s *Staker) QueuedGroupSize() (*big.Int, error) {
	return s.validations.validatorQueue.Len()
}

func (s *Staker) LeaderGroupSize() (*big.Int, error) {
	return s.validations.leaderGroup.Len()
}

// Next returns the next validator in a a linked list.
// If the provided ID is not in a list, it will return empty bytes.
func (s *Staker) Next(prev thor.Bytes32) (thor.Bytes32, error) {
	entry, err := s.Get(prev)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if entry.IsEmpty() || entry.Next == nil {
		return thor.Bytes32{}, nil
	}
	return *entry.Next, nil
}

// AddValidator queues a new validator.
func (s *Staker) AddValidator(
	endorsor thor.Address,
	master thor.Address,
	period uint32,
	stake *big.Int,
	autoRenew bool,
	currentBlock uint32,
) (thor.Bytes32, error) {
	id, err := s.validations.Add(endorsor, master, period, stake, autoRenew, currentBlock)
	if err != nil {
		return thor.Bytes32{}, err
	}
	if err := s.storage.SetDelegation(id, newDelegation()); err != nil {
		return thor.Bytes32{}, err
	}
	return id, nil
}

func (s *Staker) LookupMaster(master thor.Address) (*Validation, thor.Bytes32, error) {
	return s.storage.LookupMaster(master)
}

func (s *Staker) Get(id thor.Bytes32) (*Validation, error) {
	return s.storage.GetValidator(id)
}

func (s *Staker) UpdateAutoRenew(endorsor thor.Address, id thor.Bytes32, autoRenew bool, blockNumber uint32) error {
	return s.validations.UpdateAutoRenew(endorsor, id, autoRenew, blockNumber)
}

// IncreaseStake increases the stake of a queued or active validator
// if a validator is active, the stake is increase, but the weight stays the same
// the weight will be recalculated at the end of the staking period, by the housekeep function
func (s *Staker) IncreaseStake(endorsor thor.Address, id thor.Bytes32, amount *big.Int) error {
	return s.validations.IncreaseStake(id, endorsor, amount)
}

func (s *Staker) DecreaseStake(endorsor thor.Address, id thor.Bytes32, amount *big.Int) error {
	return s.validations.DecreaseStake(id, endorsor, amount)
}

// WithdrawStake allows expired validations to withdraw their stake.
func (s *Staker) WithdrawStake(endorsor thor.Address, id thor.Bytes32) (*big.Int, error) {
	return s.validations.WithdrawStake(endorsor, id)
}

func (s *Staker) SetOnline(id thor.Bytes32, online bool) error {
	entry, err := s.storage.GetValidator(id)
	if err != nil {
		return err
	}
	entry.Online = online
	return s.storage.SetValidator(id, entry)
}

// AddDelegator adds a new delegator.
func (s *Staker) AddDelegator(
	validationID thor.Bytes32,
	delegator thor.Address,
	stake *big.Int,
	autoRenew bool,
	multiplier uint8,
) error {
	return s.delegations.Add(validationID, delegator, stake, autoRenew, multiplier)
}

// GetDelegator returns the delegator.
func (s *Staker) GetDelegator(
	validationID thor.Bytes32,
	delegator thor.Address,
) (*Delegator, error) {
	return s.storage.GetDelegator(validationID, delegator)
}

// UpdateDelegatorAutoRenew updates the auto-renewal status of a delegator.
func (s *Staker) UpdateDelegatorAutoRenew(
	validationID thor.Bytes32,
	delegator thor.Address,
	autoRenew bool,
) error {
	if autoRenew {
		return s.delegations.EnableAutoRenew(validationID, delegator)
	}
	return s.delegations.DisableAutoRenew(validationID, delegator)
}

// DelegatorWithdrawStake allows expired delegators to withdraw their stake.
func (s *Staker) DelegatorWithdrawStake(
	id thor.Bytes32,
	delegator thor.Address,
) (*big.Int, error) {
	return s.delegations.Withdraw(id, delegator)
}

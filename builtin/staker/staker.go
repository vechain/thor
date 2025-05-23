// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"math/big"

	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/log"
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

func SetLogger(l log.Logger) {
	logger = l
}

// Staker implements native methods of `Staker` contract.
type Staker struct {
	lockedVET    *solidity.Uint256
	lockedWeight *solidity.Uint256
	queuedVET    *solidity.Uint256
	queuedWeight *solidity.Uint256
	delegations  *delegations
	validations  *validations
	storage      *storage
	params       *params.Params
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
		lockedVET:    solidity.NewUint256(addr, state, slotLockedVET),
		lockedWeight: solidity.NewUint256(addr, state, slotLockedWeight),
		queuedVET:    solidity.NewUint256(addr, state, slotQueuedVET),
		queuedWeight: solidity.NewUint256(addr, state, slotQueuedWeight),
		storage:      storage,
		validations:  newValidations(storage),
		delegations:  newDelegations(storage),
		params:       params,
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

// LockedVET returns the amount of VET and weight locked by validations and delegations.
func (s *Staker) LockedVET() (*big.Int, *big.Int, error) {
	lockedVet, err := s.lockedVET.Get()
	if err != nil {
		return nil, nil, err
	}
	lockedWeight, err := s.lockedWeight.Get()
	return lockedVet, lockedWeight, err
}

// QueuedStake returns the amount of VET and weight queued by validations and delegations.
func (s *Staker) QueuedStake() (*big.Int, *big.Int, error) {
	queuedVet, err := s.queuedVET.Get()
	if err != nil {
		return nil, nil, err
	}
	queuedWeight, err := s.queuedWeight.Get()
	return queuedVet, queuedWeight, err
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

// Next returns the next validator in a linked list.
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
	stakeETH := new(big.Int).Div(stake, big.NewInt(1e18))
	logger.Debug("adding validator", "endorsor", endorsor, "master", master, "period", period, "stake", stakeETH, "autoRenew", autoRenew)
	if id, err := s.validations.Add(endorsor, master, period, stake, autoRenew, currentBlock); err != nil {
		logger.Info("add validator failed", "master", master, "error", err)
		return thor.Bytes32{}, err
	} else {
		logger.Info("added validator", "master", master, "id", id)
		return id, nil
	}
}

func (s *Staker) LookupMaster(master thor.Address) (*Validation, thor.Bytes32, error) {
	return s.storage.LookupMaster(master)
}

func (s *Staker) Get(id thor.Bytes32) (*Validation, error) {
	return s.storage.GetValidator(id)
}

func (s *Staker) UpdateAutoRenew(endorsor thor.Address, id thor.Bytes32, autoRenew bool) error {
	logger.Debug("updating autorenew", "endorsor", endorsor, "id", id, "autoRenew", autoRenew)

	if err := s.validations.UpdateAutoRenew(endorsor, id, autoRenew); err != nil {
		logger.Info("update autorenew failed", "id", id, "error", err)
		return err
	} else {
		logger.Info("updated autorenew", "id", id)
		return nil
	}
}

// IncreaseStake increases the stake of a queued or active validator
// if a validator is active, the stake is increase, but the weight stays the same
// the weight will be recalculated at the end of the staking period, by the housekeep function
func (s *Staker) IncreaseStake(endorsor thor.Address, id thor.Bytes32, amount *big.Int) error {
	amountETH := new(big.Int).Div(amount, big.NewInt(1e18))
	logger.Debug("increasing stake", "endorsor", endorsor, "id", id, "amount", amountETH)
	if err := s.validations.IncreaseStake(id, endorsor, amount); err != nil {
		logger.Info("increase stake failed", "id", id, "error", err)
		return err
	} else {
		logger.Info("increased stake", "id", id)
		return nil
	}
}

func (s *Staker) DecreaseStake(endorsor thor.Address, id thor.Bytes32, amount *big.Int) error {
	amountETH := new(big.Int).Div(amount, big.NewInt(1e18))
	logger.Debug("decreasing stake", "endorsor", endorsor, "id", id, "amount", amountETH)

	if err := s.validations.DecreaseStake(id, endorsor, amount); err != nil {
		logger.Info("decrease stake failed", "id", id, "error", err)
		return err
	} else {
		logger.Info("decreased stake", "id", id)
		return nil
	}
}

// WithdrawStake allows expired validations to withdraw their stake.
func (s *Staker) WithdrawStake(endorsor thor.Address, id thor.Bytes32, currentBlock uint32) (*big.Int, error) {
	logger.Debug("withdrawing stake", "endorsor", endorsor, "id", id)

	if stake, err := s.validations.WithdrawStake(endorsor, id, currentBlock); err != nil {
		logger.Info("withdraw failed", "id", id, "error", err)
		return nil, err
	} else {
		logger.Info("withdrew validator staker", "id", id)
		return stake, nil
	}
}

// GetWithdrawable returns the withdrawable stake of a validator.
func (s *Staker) GetWithdrawable(id thor.Bytes32, block uint32) (*big.Int, error) {
	_, stake, err := s.validations.GetWithdrawable(id, block)
	return stake, err
}

func (s *Staker) SetOnline(id thor.Bytes32, online bool) (bool, error) {
	logger.Debug("set master online", "id", id, "online", online)
	entry, err := s.storage.GetValidator(id)
	if err != nil {
		return false, err
	}
	hasChanged := entry.Online != online
	entry.Online = online
	if hasChanged {
		err = s.storage.SetValidator(id, entry)
	} else {
		err = nil
	}
	return hasChanged, err
}

// AddDelegation adds a new delegation.
func (s *Staker) AddDelegation(
	validationID thor.Bytes32,
	stake *big.Int,
	autoRenew bool,
	multiplier uint8,
) (thor.Bytes32, error) {
	stakeETH := new(big.Int).Div(stake, big.NewInt(1e18))
	logger.Debug("adding delegation", "validationID", validationID, "stake", stakeETH, "autoRenew", autoRenew, "multiplier", multiplier)
	if id, err := s.delegations.Add(validationID, stake, autoRenew, multiplier); err != nil {
		logger.Info("failed to add delegation", "validationID", validationID, "error", err)
		return thor.Bytes32{}, err
	} else {
		weight := big.NewInt(0).Mul(stake, big.NewInt(int64(multiplier)))
		weight = big.NewInt(0).Quo(weight, big.NewInt(100))
		err := s.queuedWeight.Add(weight)
		if err != nil {
			return [32]byte{}, err
		}
		logger.Info("added delegation", "validationID", validationID, "id", id)
		return id, nil
	}
}

// GetDelegation returns the delegation.
func (s *Staker) GetDelegation(
	delegationID thor.Bytes32,
) (*Delegation, *Validation, error) {
	delegation, err := s.storage.GetDelegation(delegationID)
	if err != nil {
		return nil, nil, err
	}
	if delegation.IsEmpty() {
		return &Delegation{}, nil, nil
	}
	validation, err := s.storage.GetValidator(delegation.ValidatorID)
	if err != nil {
		return nil, nil, err
	}
	return delegation, validation, nil
}

// HasDelegations returns true if the validator has any delegations.
func (s *Staker) HasDelegations(
	master thor.Address,
) (bool, error) {
	_, validationID, err := s.storage.LookupMaster(master)
	if err != nil {
		return false, err
	}
	if validationID.IsZero() {
		return false, nil
	}
	aggregation, err := s.storage.GetAggregation(validationID)
	if err != nil {
		return false, err
	}
	if aggregation == nil || aggregation.IsEmpty() {
		return false, nil
	}
	total := new(big.Int).Add(aggregation.LockedVET, aggregation.CooldownVET)
	return total.Sign() > 0, nil
}

// UpdateDelegationAutoRenew updates the auto-renewal status of a delegation.
func (s *Staker) UpdateDelegationAutoRenew(
	delegationID thor.Bytes32,
	autoRenew bool,
) error {
	logger.Debug("updating autorenew", "delegationID", delegationID, "autoRenew", autoRenew)
	var err error
	if autoRenew {
		err = s.delegations.EnableAutoRenew(delegationID)
	} else {
		err = s.delegations.DisableAutoRenew(delegationID)
	}
	if err != nil {
		logger.Info("update autorenew failed", "delegationID", delegationID, "error", err)
	} else {
		logger.Info("updated autorenew", "delegationID", delegationID)
	}
	return err
}

// WithdrawDelegation allows expired and queued delegations to withdraw their stake.
func (s *Staker) WithdrawDelegation(
	delegationID thor.Bytes32,
) (*big.Int, error) {
	logger.Debug("withdrawing delegation", "delegationID", delegationID)
	if stake, err := s.delegations.Withdraw(delegationID); err != nil {
		logger.Info("failed to withdraw", "delegationID", delegationID, "error", err)
		return nil, err
	} else {
		stakeETH := new(big.Int).Div(stake, big.NewInt(1e18))
		logger.Info("withdrew delegation", "delegationID", delegationID, "stake", stakeETH)
		return stake, nil
	}
}

// GetRewards returns reward amount for validation id and staking period.
func (s *Staker) GetRewards(validationID thor.Bytes32, stakingPeriod uint32) (*big.Int, error) {
	return s.storage.GetRewards(validationID, stakingPeriod)
}

// GetCompletedPeriods returns number of completed staking periods for validation.
func (s *Staker) GetCompletedPeriods(validationID thor.Bytes32) (uint32, error) {
	return s.storage.GetCompletedPeriods(validationID)
}

// IncreaseReward Increases reward for master address, for current staking period.
func (s *Staker) IncreaseReward(master thor.Address, reward big.Int) error {
	return s.storage.IncreaseReward(master, reward)
}

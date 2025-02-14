// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"errors"
	"math/big"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// TODO: Do these need to be set in params.sol, or some other dynamic way?
var (
	maxLeaderGroupSize = big.NewInt(101)
	minStake           = big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	maxStake           = big.NewInt(0).Mul(big.NewInt(400e6), big.NewInt(1e18))
)

type slot = byte

const (
	slotTotalStake slot = iota
	slotActiveStake
	slotLeaderGroupSize
	slotValidators
	slotActiveHead
	slotActiveTail
	slotQueuedHead
	slotQueuedTail
)

// Staker implements native methods of `Staker` contract.
type Staker struct {
	addr            thor.Address
	state           *state.State
	totalStake      *solidity.Uint256
	activeStake     *solidity.Uint256
	leaderGroupSize *solidity.Uint256
	validators      *solidity.Mapping[thor.Address, *Validator]
	leaderGroup     *linkedList
	validatorQueue  *linkedList
}

// New create a new instance.
func New(addr thor.Address, state *state.State) *Staker {
	validators := solidity.NewMapping[thor.Address, *Validator](addr, state, thor.Bytes32{slotValidators})
	return &Staker{
		addr:            addr,
		state:           state,
		totalStake:      solidity.NewUint256(addr, state, thor.Bytes32{slotTotalStake}),
		activeStake:     solidity.NewUint256(addr, state, thor.Bytes32{slotActiveStake}),
		leaderGroupSize: solidity.NewUint256(addr, state, thor.Bytes32{slotLeaderGroupSize}),
		validators:      validators,
		leaderGroup:     newLinkedList(addr, state, validators, thor.Bytes32{slotActiveHead}, thor.Bytes32{slotActiveTail}),
		validatorQueue:  newLinkedList(addr, state, validators, thor.Bytes32{slotQueuedHead}, thor.Bytes32{slotQueuedTail}),
	}
}

// AddValidator queues a new validator.
func (a *Staker) AddValidator(addr thor.Address, stake *big.Int) error {
	if stake.Cmp(minStake) < 0 || stake.Cmp(maxStake) > 0 {
		return errors.New("stake is out of range")
	}
	entry, err := a.validators.Get(addr)
	if err != nil {
		return err
	}
	if !entry.IsEmpty() {
		return errors.New("validator already exists")
	}

	entry.Stake = stake
	entry.Weight = stake
	entry.Status = StatusQueued

	if err := a.validatorQueue.Add(entry, addr); err != nil {
		return err
	}
	if err := a.totalStake.Add(stake); err != nil {
		return err
	}

	return nil
}

func (a *Staker) Get(validator thor.Address) (*Validator, error) {
	return a.validators.Get(validator)
}

// ActivateNextValidator pops the head of the queue and adds it to the leader group.
// It will also increase the active stake
// If there is no validator in the queue, it will return an error.
func (a *Staker) ActivateNextValidator() error {
	leaderGroupLength, err := a.leaderGroupSize.Get()
	if err != nil {
		return err
	}
	if leaderGroupLength.Cmp(maxLeaderGroupSize) >= 0 {
		return errors.New("leader group is full")
	}
	if err := a.leaderGroupSize.Add(big.NewInt(1)); err != nil {
		return nil
	}
	// pop the head of the queue
	validator, addr, err := a.validatorQueue.Pop()
	if err != nil {
		return err
	}
	if validator.IsEmpty() {
		return errors.New("no validator in the queue")
	}
	if err := a.activeStake.Add(validator.Stake); err != nil {
		return err
	}
	validator.Status = StatusActive
	// add to the active list
	if err := a.leaderGroup.Add(validator, addr); err != nil {
		return err
	}

	return nil
}

// RemoveValidator sets a validators status to exited and removes it from the active list.
// It will also decrease the total stake. Exited validators can then withdraw their stake.
func (a *Staker) RemoveValidator(validator thor.Address) error {
	entry, err := a.validators.Get(validator)
	if err != nil {
		return err
	}
	if entry.Status != StatusActive {
		return errors.New("validator is not active")
	}

	if err := a.totalStake.Sub(entry.Stake); err != nil {
		return err
	}
	if err := a.activeStake.Sub(entry.Stake); err != nil {
		return err
	}

	entry.Weight = big.NewInt(0)
	entry.Status = StatusExit

	return a.leaderGroup.Remove(validator, entry)
}

// LeaderGroup lists all registered candidates.
func (a *Staker) LeaderGroup() (map[thor.Address]*Validator, error) {
	ptr, err := a.FirstActive()
	if err != nil {
		return nil, err
	}
	var group = make(map[thor.Address]*Validator)
	for {
		entry, err := a.validators.Get(ptr)
		if err != nil {
			return nil, err
		}
		group[ptr] = entry
		if entry.Next == nil || entry.Next.IsZero() {
			break
		}
		ptr = *entry.Next
	}
	return group, nil
}

func (a *Staker) TotalStake() (*big.Int, error) {
	return a.totalStake.Get()
}

func (a *Staker) ActiveStake() (*big.Int, error) {
	return a.activeStake.Get()
}

// FirstActive returns validator address of first entry.
func (a *Staker) FirstActive() (thor.Address, error) {
	return a.leaderGroup.head.Get()
}

// FirstQueued returns validator address of first entry.
func (a *Staker) FirstQueued() (thor.Address, error) {
	return a.validatorQueue.head.Get()
}

// Next returns the next validator in a a linked list.
// If the provided address is not in a list, it will return a zero address.
func (a *Staker) Next(prev thor.Address) (*thor.Address, error) {
	entry, err := a.validators.Get(prev)
	if err != nil {
		return nil, err
	}
	if entry.IsEmpty() {
		return nil, errors.New("provided address is not in a linked list")
	}
	return entry.Next, nil
}

// GetStake returns the stake of a validator.
func (a *Staker) GetStake(validator thor.Address) (*big.Int, error) {
	entry, err := a.validators.Get(validator)
	if err != nil {
		return nil, err
	}
	return entry.Stake, nil
}

// WithdrawStake allows expired validators to withdraw their stake.
func (a *Staker) WithdrawStake(validator thor.Address) (*big.Int, error) {
	entry, err := a.validators.Get(validator)
	if err != nil {
		return nil, err
	}
	if entry.IsEmpty() {
		return nil, nil
	}
	if entry.Status != StatusExit {
		return nil, errors.New("validator is not inactive")
	}
	if err := a.totalStake.Sub(entry.Stake); err != nil {
		return nil, err
	}
	if err := a.validators.Set(validator, &Validator{}); err != nil {
		return nil, err
	}
	return entry.Stake, nil
}

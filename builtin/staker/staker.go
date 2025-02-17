// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"errors"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// TODO: Do these need to be set in params.sol, or some other dynamic way?
var (
	maxLeaderGroupSize = big.NewInt(101)
	minStake           = big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	maxStake           = big.NewInt(0).Mul(big.NewInt(400e6), big.NewInt(1e18))

	minStakingPeriod = uint64(360) * 24 * 14  // 2 weeks
	maxStakingPeriod = uint64(360) * 24 * 365 // 1 year
	_                = uint64(360) * 24 * 1   // 1 day

	slotPreviousExitKey = thor.Blake2b(thor.Bytes32{slotPreviousExit}.Bytes())
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
	slotPreviousExit
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
func (a *Staker) AddValidator(
	currentBlock uint64,
	addr thor.Address,
	beneficiary thor.Address,
	expiry uint64,
	stake *big.Int,
) error {
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

	period := expiry - currentBlock
	if expiry <= currentBlock ||
		period < minStakingPeriod ||
		period > maxStakingPeriod {
		return errors.New("expiry is out of boundaries")
	}

	entry.Stake = stake
	entry.Weight = stake
	entry.Status = StatusQueued
	entry.Beneficiary = beneficiary
	entry.Expiry = expiry

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

func (a *Staker) PreviousExit() (uint64, error) {
	value := previousExit{}
	err := a.state.DecodeStorage(a.addr, slotPreviousExitKey, func(raw []byte) error {
		if len(raw) == 0 {
			return nil
		}
		return rlp.DecodeBytes(raw, &value)
	})
	return value.PreviousExit, err
}

func (a *Staker) setPreviousExit(blockNumber uint64) error {
	value := previousExit{
		PreviousExit: blockNumber,
	}
	return a.state.EncodeStorage(a.addr, slotPreviousExitKey, func() ([]byte, error) {
		return rlp.EncodeToBytes(value)
	})
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

// Iterate over validators, move to cooldown
// take the oldest validator and move to exited
func (a *Staker) Housekeep(currentBlock uint64) error {
	ptr, err := a.FirstActive()
	if err != nil {
		return err
	}
	next := &ptr

	toExit := thor.Address{}
	toExitExp := uint64(math.MaxUint64)
	for next != nil {
		entry, err := a.validators.Get(*next)
		if err != nil {
			return err
		}

		if currentBlock >= entry.Expiry {
			if entry.Status == StatusActive {
				// Put to cooldown
				entry.Status = StatusCooldown
				if err := a.validators.Set(*next, entry); err != nil {
					return err
				}
			}

			// Find calidator with the lowest expiry
			if entry.Status == StatusCooldown && toExitExp > entry.Expiry {
				toExit = *next
				toExitExp = entry.Expiry
			}
		}

		next = entry.Next
	}

	// should the protocol handle this case? `((currentBlock-forkBlock)%cooldownPeriod) == 0`
	if !toExit.IsZero() {
		if err := a.RemoveValidator(toExit); err != nil {
			return err
		}
		if err := a.setPreviousExit(currentBlock); err != nil {
			return err
		}
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

	if entry.IsEmpty() {
		return nil
	}

	// not sure if this check is even needed
	//if entry.Status != StatusCooldown {
	//	return errors.New("validator is not on cooldown")
	//}

	if err := a.totalStake.Sub(entry.Stake); err != nil {
		return err
	}
	if err := a.activeStake.Sub(entry.Stake); err != nil {
		return err
	}

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

func IsExitingSlot(blockNumber uint64) bool {
	if blockNumber == math.MaxUint64 {
		return false
	}
	return (((blockNumber+1)/180)*180)-1 == blockNumber
}

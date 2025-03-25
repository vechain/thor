// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"errors"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/builtin/authority"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// TODO: Do these need to be set in params.sol, or some other dynamic way?
var (
	minStake = big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))
	maxStake = big.NewInt(0).Mul(big.NewInt(400e6), big.NewInt(1e18))

	// TODO: Enable these once customnet testing is done
	//oneWeekStakingPeriod  = uint32(360) * 24 * 7     // 1 weeks
	//twoWeeksStakingPeriod = oneWeekStakingPeriod * 2 // 2 weeks
	//oneMonthStakingPeriod = uint32(360) * 24 * 30    // 30 days

	slotPreviousExitKey = thor.Blake2b(thor.Bytes32{slotPreviousExit}.Bytes())
	cooldownPeriod      = uint32(8640)
)

type slot = byte

const (
	slotTotalStake slot = iota
	slotActiveStake
	slotLeaderGroupSize
	slotMaxLeaderGroupSize
	slotValidators
	slotActiveHead
	slotActiveTail
	slotQueuedHead
	slotQueuedTail
	slotPreviousExit
	slotQueuedGroupSize
)

// Staker implements native methods of `Staker` contract.
type Staker struct {
	addr               thor.Address
	state              *state.State
	totalStake         *solidity.Uint256
	activeStake        *solidity.Uint256
	leaderGroupSize    *solidity.Uint256
	maxLeaderGroupSize *solidity.Uint256
	validators         *solidity.Mapping[thor.Address, *Validator]
	leaderGroup        *linkedList
	validatorQueue     *orderedLinkedList
	queuedGroupSize    *solidity.Uint256 // New field for tracking queued validators count
	// TODO: remove once customnet testing is done https://github.com/vechain/protocol-board-repo/issues/486
	lowStakingPeriod    uint32
	mediumStakingPeriod uint32
	highStakingPeriod   uint32
	epochLength         uint32
}

// New create a new instance.
func New(addr thor.Address, state *state.State) *Staker {
	validators := solidity.NewMapping[thor.Address, *Validator](addr, state, thor.Bytes32{slotValidators})

	lowStakingPeriod := uint32(360) * 24 * 7
	if num, err := solidity.NewUint256(addr, state, thor.BytesToBytes32([]byte("staker-low-staking-period"))).Get(); err == nil {
		numUint64 := num.Uint64()
		if numUint64 != 0 {
			lowStakingPeriod = uint32(numUint64)
		}
	}
	mediumStakingPeriod := uint32(360) * 24 * 14
	if num, err := solidity.NewUint256(addr, state, thor.BytesToBytes32([]byte("staker-medium-staking-period"))).Get(); err == nil {
		numUint64 := num.Uint64()
		if numUint64 != 0 {
			mediumStakingPeriod = uint32(numUint64)
		}
	}
	highStakingPeriod := uint32(360) * 24 * 30
	if num, err := solidity.NewUint256(addr, state, thor.BytesToBytes32([]byte("staker-high-staking-period"))).Get(); err == nil {
		numUint64 := num.Uint64()
		if numUint64 != 0 {
			highStakingPeriod = uint32(numUint64)
		}
	}

	epochLength := uint32(180)
	if num, err := solidity.NewUint256(addr, state, thor.BytesToBytes32([]byte("epoch-length"))).Get(); err == nil {
		numUint64 := num.Uint64()
		if numUint64 != 0 {
			epochLength = uint32(numUint64)
		}
	}

	return &Staker{
		addr:                addr,
		state:               state,
		totalStake:          solidity.NewUint256(addr, state, thor.Bytes32{slotTotalStake}),
		activeStake:         solidity.NewUint256(addr, state, thor.Bytes32{slotActiveStake}),
		leaderGroupSize:     solidity.NewUint256(addr, state, thor.Bytes32{slotLeaderGroupSize}),
		validators:          validators,
		maxLeaderGroupSize:  solidity.NewUint256(addr, state, thor.Bytes32{slotMaxLeaderGroupSize}),
		leaderGroup:         newLinkedList(addr, state, validators, thor.Bytes32{slotActiveHead}, thor.Bytes32{slotActiveTail}),
		validatorQueue:      newOrderedLinkedList(addr, state, validators, thor.Bytes32{slotQueuedHead}, thor.Bytes32{slotQueuedTail}),
		queuedGroupSize:     solidity.NewUint256(addr, state, thor.Bytes32{slotQueuedGroupSize}),
		epochLength:         epochLength,
		lowStakingPeriod:    lowStakingPeriod,
		mediumStakingPeriod: mediumStakingPeriod,
		highStakingPeriod:   highStakingPeriod,
	}
}

func (a *Staker) SetOnline(master thor.Address, online bool) error {
	entry, err := a.validators.Get(master)
	if err != nil {
		return err
	}
	entry.Online = online
	return a.validators.Set(master, entry)
}

// AddValidator queues a new validator.
func (a *Staker) AddValidator(
	endorsor thor.Address,
	master thor.Address,
	period uint32,
	stake *big.Int,
	autoRenew bool,
) error {
	if stake.Cmp(minStake) < 0 || stake.Cmp(maxStake) > 0 {
		return errors.New("stake is out of range")
	}
	entry, err := a.validators.Get(master)
	if err != nil {
		return err
	}
	if !entry.IsEmpty() {
		return errors.New("validator already exists")
	}

	if period != a.lowStakingPeriod && period != a.mediumStakingPeriod && period != a.highStakingPeriod {
		return errors.New("period is out of boundaries")
	}

	entry.Stake = stake
	entry.Weight = stake
	entry.Status = StatusQueued
	entry.Expiry = nil
	entry.Period = period
	entry.Endorsor = endorsor
	entry.AutoRenew = autoRenew

	if err := a.validatorQueue.Add(master, entry); err != nil {
		return err
	}
	if err := a.totalStake.Add(stake); err != nil {
		return err
	}

	// Increment queuedGroupSize when adding validator to queue
	if err := a.queuedGroupSize.Add(big.NewInt(1)); err != nil {
		return err
	}

	return nil
}

func (a *Staker) UpdateAutoRenew(endorsor thor.Address, master thor.Address, autoRenew bool, blockNumber uint32) error {
	validator, err := a.validators.Get(master)
	if err != nil {
		return err
	}
	if validator.IsEmpty() {
		return errors.New("validator not found")
	}
	if validator.Endorsor != endorsor {
		return errors.New("invalid endorsor for master")
	}
	validator.AutoRenew = autoRenew
	if !autoRenew {
		validator.ExitTxBlock = blockNumber
	}
	return a.validators.Set(master, validator)
}

func (a *Staker) IncreaseStake(master thor.Address, endorsor thor.Address, amount *big.Int) (*big.Int, error) {
	entry, err := a.validators.Get(master)
	if err != nil {
		return nil, err
	}
	if entry.IsEmpty() {
		return nil, errors.New("validator doesn't exist")
	}
	if entry.Endorsor != endorsor {
		return nil, errors.New("invalid endorser")
	}
	if entry.Status != StatusQueued {
		return nil, errors.New("validator is not queued")
	}

	newStake := big.NewInt(0).Add(entry.Stake, amount)
	if newStake.Cmp(maxStake) > 0 {
		return nil, errors.New("stake is out of range")
	}

	if _, err := a.WithdrawStake(endorsor, master); err != nil {
		return nil, errors.New("unable to withdraw validator")
	}

	entry.Stake = newStake
	entry.Weight = newStake

	if err := a.validatorQueue.Add(master, entry); err != nil {
		return nil, err
	}
	if err := a.queuedGroupSize.Add(big.NewInt(1)); err != nil {
		return nil, err
	}

	return newStake, nil
}

func (a *Staker) Get(master thor.Address) (*Validator, error) {
	return a.validators.Get(master)
}

func (a *Staker) PreviousExit() (uint32, error) {
	value := previousExit{}
	err := a.state.DecodeStorage(a.addr, slotPreviousExitKey, func(raw []byte) error {
		if len(raw) == 0 {
			return nil
		}
		return rlp.DecodeBytes(raw, &value)
	})
	return value.PreviousExit, err
}

func (a *Staker) setPreviousExit(blockNumber uint32) error {
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
func (a *Staker) ActivateNextValidator(blockNumber uint32) error {
	leaderGroupLength, err := a.leaderGroupSize.Get()
	if err != nil {
		return err
	}
	maxLeaderGroupSize, err := a.maxLeaderGroupSize.Get()
	if err != nil {
		return err
	}
	if leaderGroupLength.Cmp(maxLeaderGroupSize) >= 0 {
		return errors.New("leader group is full")
	}

	// Check if queue is empty
	queuedSize, err := a.queuedGroupSize.Get()
	if err != nil {
		return err
	}
	if queuedSize.Cmp(big.NewInt(0)) <= 0 {
		return errors.New("no validator in the queue")
	}

	if err := a.leaderGroupSize.Add(big.NewInt(1)); err != nil {
		return nil
	}
	// pop the head of the queue
	addr, validator, err := a.validatorQueue.Pop()
	if err != nil {
		return err
	}
	if validator.IsEmpty() {
		return errors.New("no validator in the queue")
	}

	// Decrement queuedGroupSize when removing from queue
	if err := a.queuedGroupSize.Sub(big.NewInt(1)); err != nil {
		return err
	}

	if err := a.activeStake.Add(validator.Stake); err != nil {
		return err
	}

	expiry := blockNumber + validator.Period
	validator.Status = StatusActive
	validator.Online = true
	validator.Expiry = &expiry
	validator.ExitTxBlock = blockNumber
	// add to the active list
	if err := a.leaderGroup.Add(addr, validator); err != nil {
		return err
	}

	return nil
}

// Housekeep iterates over validators, move to cooldown
// take the oldest validator and move to exited
func (a *Staker) Housekeep(currentBlock uint32, forkBlock uint32) (bool, error) {
	validatorsChanged := false
	if (currentBlock-forkBlock)%a.epochLength == 0 {
		ptr, err := a.FirstActive()
		if err != nil {
			return false, err
		}
		next := &ptr

		toExit := thor.Address{}
		toExitExp := uint32(math.MaxUint32)
		for next != nil && !next.IsZero() {
			entry, err := a.validators.Get(*next)
			if err != nil {
				return false, err
			}

			if entry.Expiry != nil && currentBlock >= *entry.Expiry {
				if entry.Status == StatusActive && !entry.AutoRenew {
					// Put to cooldown
					entry.Status = StatusCooldown
					if err := a.validators.Set(*next, entry); err != nil {
						return false, err
					}
				} else if entry.Status == StatusActive && entry.AutoRenew {
					// Renew the validator
					expiry := *entry.Expiry + entry.Period
					entry.Expiry = &expiry
					if err := a.validators.Set(*next, entry); err != nil {
						return false, err
					}
				}

				// Find validator with the lowest exit tx block
				if entry.Status == StatusCooldown && toExitExp > entry.ExitTxBlock && currentBlock >= *entry.Expiry+cooldownPeriod {
					toExit = *next
					toExitExp = entry.ExitTxBlock
				}
			}

			next = entry.Next
		}

		// should the protocol handle this case? `((currentBlock-forkBlock)%cooldownPeriod) == 0`
		if !toExit.IsZero() && currentBlock%a.epochLength == 0 {
			validatorsChanged = true
			if err := a.RemoveValidator(toExit, currentBlock); err != nil {
				return false, err
			}
			if err := a.setPreviousExit(currentBlock); err != nil {
				return false, err
			}
		}
	}

	if currentBlock%a.epochLength == 0 {
		validatorsChanged = true
		if err := a.activateValidators(currentBlock); err != nil {
			return false, err
		}
	}

	return validatorsChanged, nil
}
func (a *Staker) activateValidators(currentBlock uint32) error {
	queuedSize, err := a.QueuedGroupSize()
	if err != nil {
		return err
	}
	leaderSize, err := a.LeaderGroupSize()
	if err != nil {
		return err
	}

	if queuedSize.Cmp(big.NewInt(0)) > 0 {
		queuedCount := queuedSize.Int64()
		leaderDelta := 101 - leaderSize.Int64()
		if leaderDelta > 0 {
			if leaderDelta < queuedCount {
				queuedCount = leaderDelta
			}
		} else {
			queuedCount = 0
		}

		for i := int64(0); i < queuedCount; i++ {
			err := a.ActivateNextValidator(currentBlock)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// RemoveValidator sets a validators status to exited and removes it from the active list.
// It will also decrease the total stake. Exited validators can then withdraw their stake.
func (a *Staker) RemoveValidator(master thor.Address, currentBlock uint32) error {
	entry, err := a.validators.Get(master)
	if err != nil {
		return err
	}

	if entry.IsEmpty() {
		return nil
	}

	if entry.Status != StatusExit && *entry.Expiry > currentBlock {
		return errors.New("validator cannot be removed")
	}

	if err := a.totalStake.Sub(entry.Stake); err != nil {
		return err
	}
	if err := a.activeStake.Sub(entry.Stake); err != nil {
		return err
	}

	entry.Status = StatusExit
	entry.Weight = big.NewInt(0)

	return a.leaderGroup.Remove(master, entry)
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
		if entry.IsEmpty() {
			break
		}
		group[ptr] = entry
		if entry.Next == nil || entry.Next.IsZero() {
			break
		}
		ptr = *entry.Next
	}
	return group, nil
}

func (a *Staker) EpochLength() uint32 {
	return a.epochLength
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
	return a.validatorQueue.linkedList.head.Get()
}

// Next returns the next validator in a a linked list.
// If the provided address is not in a list, it will return a zero address.
func (a *Staker) Next(prev thor.Address) (thor.Address, error) {
	entry, err := a.Get(prev)
	if err != nil {
		return thor.Address{}, err
	}
	if entry.IsEmpty() || entry.Next == nil {
		return thor.Address{}, nil
	}
	return *entry.Next, nil
}

// GetStake returns the stake of a validator.
func (a *Staker) GetStake(master thor.Address) (*big.Int, error) {
	entry, err := a.validators.Get(master)
	if err != nil {
		return nil, err
	}
	return entry.Stake, nil
}

// WithdrawStake allows expired validators to withdraw their stake.
func (a *Staker) WithdrawStake(endorsor thor.Address, master thor.Address) (*big.Int, error) {
	entry, err := a.validators.Get(master)
	if err != nil {
		return nil, err
	}
	if entry.IsEmpty() {
		return big.NewInt(0), nil
	}
	if entry.Endorsor != endorsor {
		return big.NewInt(0), errors.New("invalid endorser")
	}
	switch entry.Status {
	case StatusExit:
		// skip
	case StatusQueued:
		if err := a.validatorQueue.Remove(master, entry); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("validator is not inactive")
	}
	if err := a.validators.Set(master, &Validator{}); err != nil {
		return nil, err
	}
	return entry.Stake, nil
}

// Initialise the staker contract with the PoA candidates.
func (a *Staker) Initialise(auth *authority.Authority, params *params.Params, currentBlock uint32) error {
	currentSize, err := a.leaderGroupSize.Get()
	if err != nil {
		return err
	}
	if currentSize.Cmp(big.NewInt(0)) != 0 {
		// TODO: Runtime has small edge case/bug for accounts/debug endpoints. Runtime is accepting state of completed
		// block N and block context of N. Block context should have been N+1 to avoid this issue.
		// Once we decide how we will bootstrap the Staker contract we should resolve this issue
		return nil
	}

	// init max validators
	maxProposers, err := params.Get(thor.KeyMaxBlockProposers)
	if err != nil || maxProposers.Cmp(big.NewInt(0)) == 0 {
		maxProposers = big.NewInt(0).SetUint64(thor.InitialMaxBlockProposers)
	}
	a.maxLeaderGroupSize.Set(maxProposers)

	// init validators
	stake := big.NewInt(0) // validators have soft staked minimum 25M VET
	weight := big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18))

	poaCandidates, err := auth.AllCandidates()
	if err != nil {
		panic(err)
	}

	var previous *thor.Address
	expiry := currentBlock + a.mediumStakingPeriod
	for i, candidate := range poaCandidates {
		validator := &Validator{
			Expiry:   &expiry,
			Period:   a.mediumStakingPeriod,
			Stake:    stake,
			Weight:   weight,
			Status:   StatusActive,
			Prev:     previous,
			Online:   candidate.Active,
			Endorsor: candidate.Endorsor,
		}
		if i < len(poaCandidates)-1 {
			validator.Next = &poaCandidates[i+1].NodeMaster
		}
		if ok, err := auth.Revoke(candidate.NodeMaster); err != nil || !ok {
			return errors.New("failed to revoke authority candidate")
		}
		if err := a.leaderGroup.Add(candidate.NodeMaster, validator); err != nil {
			return err
		}

		previous = &candidate.NodeMaster
	}

	total := big.NewInt(0).Mul(weight, big.NewInt(int64(len(poaCandidates))))
	a.activeStake.Set(total)
	a.totalStake.Set(total)
	a.leaderGroupSize.Set(big.NewInt(int64(len(poaCandidates))))

	return nil
}

// QueuedGroupSize returns the number of validators in the queue
func (a *Staker) QueuedGroupSize() (*big.Int, error) {
	return a.queuedGroupSize.Get()
}
func (a *Staker) LeaderGroupSize() (*big.Int, error) {
	return a.leaderGroupSize.Get()
}

func IsExitingSlot(blockNumber uint64) bool {
	if blockNumber == math.MaxUint64 {
		return false
	}
	return (((blockNumber+1)/180)*180)-1 == blockNumber
}

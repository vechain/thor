// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"encoding/binary"
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

var (
	slotExitEpochs  = thor.BytesToBytes32([]byte(("exit-epochs")))
	slotValidations = thor.BytesToBytes32([]byte(("validations")))
	slotRewards     = thor.BytesToBytes32([]byte(("period-rewards")))

	// active validations linked list
	slotActiveHead      = thor.BytesToBytes32([]byte(("validations-active-head")))
	slotActiveTail      = thor.BytesToBytes32([]byte(("validations-active-tail")))
	slotActiveGroupSize = thor.BytesToBytes32([]byte(("validations-active-group-size")))

	// queued validations linked list
	slotQueuedHead      = thor.BytesToBytes32([]byte(("validations-queued-head")))
	slotQueuedTail      = thor.BytesToBytes32([]byte(("validations-queued-tail")))
	slotQueuedGroupSize = thor.BytesToBytes32([]byte(("validations-queued-group-size")))
)

type Repository struct {
	validations *solidity.Mapping[thor.Address, Validation]
	rewards     *solidity.Mapping[thor.Bytes32, *big.Int] // stores rewards per validator staking period

	exits *solidity.Mapping[thor.Bytes32, thor.Address] // exit block -> validator ID

	activeList *listStats // active list stats
	queuedList *listStats // queued list stats
}

func NewRepository(sctx *solidity.Context) *Repository {
	return &Repository{
		validations: solidity.NewMapping[thor.Address, Validation](sctx, slotValidations),
		rewards:     solidity.NewMapping[thor.Bytes32, *big.Int](sctx, slotRewards),
		exits:       solidity.NewMapping[thor.Bytes32, thor.Address](sctx, slotExitEpochs),

		activeList: newListStats(sctx, slotActiveHead, slotActiveTail, slotActiveGroupSize),
		queuedList: newListStats(sctx, slotQueuedHead, slotQueuedTail, slotQueuedGroupSize),
	}
}

func (r *Repository) getValidation(validator thor.Address) (*Validation, error) {
	v, err := r.validations.Get(validator)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator")
	}
	return &v, nil
}

func (r *Repository) addValidation(validator thor.Address, entry *Validation) error {
	if err := r.addQueued(validator, entry); err != nil {
		return errors.Wrap(err, "failed to add validator to queued list")
	}
	return nil
}

func (r *Repository) updateValidation(validator thor.Address, entry *Validation) error {
	if err := r.validations.Set(validator, *entry, false); err != nil {
		return errors.Wrap(err, "failed to set validator")
	}
	return nil
}

func (r *Repository) getReward(key thor.Bytes32) (*big.Int, error) {
	reward, err := r.rewards.Get(key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reward")
	}
	if reward == nil {
		return new(big.Int), nil
	}
	return reward, nil
}

func (r *Repository) setReward(key thor.Bytes32, val *big.Int, isNew bool) error {
	return r.rewards.Set(key, val, isNew)
}

func (r *Repository) getExit(block uint32) (thor.Address, error) {
	var key thor.Bytes32
	binary.BigEndian.PutUint32(key[:], block)

	return r.exits.Get(key)
}

func (r *Repository) setExit(block uint32, validator thor.Address) error {
	var key thor.Bytes32
	binary.BigEndian.PutUint32(key[:], block)

	if err := r.exits.Set(key, validator, true); err != nil {
		return errors.Wrap(err, "failed to set exit epoch")
	}
	return nil
}

// linked list operation
func (r *Repository) firstActive() (thor.Address, error) {
	return thor.Address{}, nil
}

func (r *Repository) firstQueued() (thor.Address, error) {
	return thor.Address{}, nil
}

func (r *Repository) nextEntry(prev thor.Address) (thor.Address, error) {
	val, err := r.validations.Get(prev)
	if err != nil {
		return thor.Address{}, errors.Wrap(err, "failed to get next")
	}

	if val.Next == nil {
		return thor.Address{}, nil
	}
	return *val.Next, nil
}

func (r *Repository) activeListSize() (uint64, error) {
	return r.activeList.GetSize()
}

func (r *Repository) queuedListSize() (uint64, error) {
	return r.queuedList.GetSize()
}

func (r *Repository) queuedListHead() (thor.Address, error) {
	head, err := r.queuedList.GetHead()
	if err != nil {
		return thor.Address{}, err
	}

	if head == nil {
		return thor.Address{}, nil
	}
	return *head, nil
}

func (r *Repository) activeListHead() (thor.Address, error) {
	head, err := r.activeList.GetHead()
	if err != nil {
		return thor.Address{}, err
	}

	if head == nil {
		return thor.Address{}, nil
	}
	return *head, nil
}

func (r *Repository) addQueued(address thor.Address, newEntry *Validation) error {
	return addToList(r, r.queuedList, address, newEntry)
}

func (r *Repository) popQueued() (thor.Address, *Validation, error) {
	head, err := r.queuedList.GetHead()
	if err != nil {
		return thor.Address{}, nil, errors.New("no head present")
	}

	if head == nil {
		return thor.Address{}, nil, errors.New("list is empty")
	}

	entry, err := r.getValidation(*head)
	if err != nil {
		return thor.Address{}, nil, err
	}
	if entry.IsEmpty() {
		return thor.Address{}, nil, errors.New("entry is empty")
	}

	// otherwise, remove and return
	val, err := removeFromList(r, r.queuedList, *head, entry)
	if err != nil {
		return thor.Address{}, nil, err
	}
	return *head, val, nil
}

// removeQueued removes the entry from the queued list and persists the entry
func (r *Repository) removeQueued(address thor.Address, entry *Validation) error {
	_, err := removeFromList(r, r.queuedList, address, entry)
	return err
}

// addActive adds the entry to the active list and persists the entry
func (r *Repository) addActive(address thor.Address, newEntry *Validation) error {
	return addToList(r, r.activeList, address, newEntry)
}

// removeActive removes the entry from the active list and persists the entry
func (r *Repository) removeActive(address thor.Address, entry *Validation) error {
	_, err := removeFromList(r, r.activeList, address, entry)
	return err
}

func (r *Repository) iterateActive(callbacks ...func(thor.Address, *Validation) error) error {
	current, err := r.activeList.GetHead()
	if err != nil {
		return err
	}

	for current != nil {
		entry, err := r.getValidation(*current)
		if err != nil {
			return err
		}
		if entry.IsEmpty() {
			return errors.New("entry is empty")
		}
		for _, callback := range callbacks {
			if err := callback(*current, entry); err != nil {
				return err
			}
		}
		current = entry.Next
	}

	return nil
}

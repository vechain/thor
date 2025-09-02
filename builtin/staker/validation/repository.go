// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

var (
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
	storage    *Storage
	activeList *listStats // active list stats
	queuedList *listStats // queued list stats
}

func NewRepository(sctx *solidity.Context) *Repository {
	storage := NewStorage(sctx)
	return &Repository{
		storage:    storage,
		activeList: newListStats(sctx, storage, slotActiveHead, slotActiveTail, slotActiveGroupSize, false),
		queuedList: newListStats(sctx, storage, slotQueuedHead, slotQueuedTail, slotQueuedGroupSize, true),
	}
}

func (r *Repository) getValidation(validator thor.Address) (*Validation, error) {
	return r.storage.getValidation(validator)
}

func (r *Repository) addValidation(validator thor.Address, entry *Validation) error {
	if err := r.queuedList.add(validator, entry); err != nil {
		return errors.Wrap(err, "failed to add validator to queued list")
	}
	return nil
}

func (r *Repository) updateValidation(validator thor.Address, entry *Validation) error {
	return r.storage.updateValidation(validator, entry)
}

func (r *Repository) getReward(key thor.Bytes32) (*big.Int, error) {
	return r.storage.getReward(key)
}

func (r *Repository) setReward(key thor.Bytes32, val *big.Int, isNew bool) error {
	return r.storage.setReward(key, val, isNew)
}

func (r *Repository) getExit(block uint32) (thor.Address, error) {
	return r.storage.getExit(block)
}

func (r *Repository) setExit(block uint32, validator thor.Address) error {
	return r.storage.setExit(block, validator)
}

// linked list operation
func (r *Repository) firstQueued() (thor.Address, error) {
	head, err := r.queuedList.getHead()
	if err != nil {
		return thor.Address{}, err
	}

	if head == nil {
		return thor.Address{}, nil
	}
	return *head, nil
}

func (r *Repository) firstActive() (thor.Address, error) {
	head, err := r.activeList.getHead()
	if err != nil {
		return thor.Address{}, err
	}

	if head == nil {
		return thor.Address{}, nil
	}
	return *head, nil
}

func (r *Repository) nextEntry(prev thor.Address) (thor.Address, error) {
	val, err := r.storage.getValidation(prev)
	if err != nil {
		return thor.Address{}, errors.Wrap(err, "failed to get next")
	}

	if val.Next == nil {
		return thor.Address{}, nil
	}
	return *val.Next, nil
}

func (r *Repository) activeListSize() (uint64, error) {
	return r.activeList.getSize()
}

func (r *Repository) queuedListSize() (uint64, error) {
	return r.queuedList.getSize()
}

func (r *Repository) popQueued() (thor.Address, *Validation, error) {
	head, err := r.queuedList.getHead()
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
	val, err := r.queuedList.remove(*head, entry)
	if err != nil {
		return thor.Address{}, nil, err
	}
	return *head, val, nil
}

// removeQueued removes the entry from the queued list and persists the entry
func (r *Repository) removeQueued(address thor.Address, entry *Validation) error {
	_, err := r.queuedList.remove(address, entry)
	return err
}

// addActive adds the entry to the active list and persists the entry
func (r *Repository) addActive(address thor.Address, newEntry *Validation) error {
	return r.activeList.add(address, newEntry)
}

// removeActive removes the entry from the active list and persists the entry
func (r *Repository) removeActive(address thor.Address, entry *Validation) error {
	_, err := r.activeList.remove(address, entry)
	return err
}

func (r *Repository) iterateActive(callbacks ...func(thor.Address, *Validation) error) error {
	current, err := r.activeList.getHead()
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

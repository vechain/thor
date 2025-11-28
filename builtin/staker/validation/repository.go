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

	// update list
	slotRenewalHead = thor.BytesToBytes32([]byte(("validations-renewal-head")))
	slotRenewalTail = thor.BytesToBytes32([]byte(("validations-renewal-tail")))
	slotRenewalPrev = thor.BytesToBytes32([]byte(("validations-renewal-prev")))
	slotRenewalNext = thor.BytesToBytes32([]byte(("validations-renewal-next")))
)

type Repository struct {
	storage     *Storage
	activeList  *listStats   // active list stats
	queuedList  *listStats   // queued list stats
	renewalList *renewalList // renewal list
}

func NewRepository(sctx *solidity.Context) *Repository {
	storage := NewStorage(sctx)
	return &Repository{
		storage:     storage,
		activeList:  newListStats(sctx, storage, slotActiveHead, slotActiveTail, slotActiveGroupSize),
		queuedList:  newListStats(sctx, storage, slotQueuedHead, slotQueuedTail, slotQueuedGroupSize),
		renewalList: newRenewalList(sctx, slotRenewalHead, slotRenewalTail, slotRenewalPrev, slotRenewalNext),
	}
}

func (r *Repository) getValidation(validator thor.Address) (*Validation, error) {
	return r.storage.getValidation(validator)
}

func (r *Repository) addValidation(validator thor.Address, entry *Validation) error {
	if err := r.queuedList.Add(validator, entry); err != nil {
		return errors.Wrap(err, "failed to add validator to queued list")
	}
	return nil
}

func (r *Repository) updateValidation(validator thor.Address, entry *Validation) error {
	if err := r.storage.updateValidation(validator, entry); err != nil {
		return errors.Wrap(err, "failed to set validator")
	}
	return nil
}

func (r *Repository) getReward(key thor.Bytes32) (*big.Int, error) {
	return r.storage.getReward(key)
}

func (r *Repository) setReward(key thor.Bytes32, val *big.Int) error {
	return r.storage.setReward(key, val)
}

func (r *Repository) getExit(block uint32) (thor.Address, error) {
	return r.storage.getExit(block)
}

func (r *Repository) setExit(block uint32, validator thor.Address) error {
	return r.storage.setExit(block, validator)
}

// linked list operation
func (r *Repository) firstQueued() (thor.Address, error) {
	return r.queuedList.GetHead()
}

func (r *Repository) firstActive() (thor.Address, error) {
	return r.activeList.GetHead()
}

func (r *Repository) nextEntry(prev thor.Address) (thor.Address, error) {
	val, err := r.storage.getValidation(prev)
	if err != nil {
		return thor.Address{}, errors.Wrap(err, "failed to get next")
	}

	if val == nil {
		return thor.Address{}, nil // not found, just return empty
	}
	if val.body.Next == nil {
		return thor.Address{}, nil
	}
	return *val.body.Next, nil
}

func (r *Repository) activeListSize() (uint64, error) {
	return r.activeList.GetSize()
}

func (r *Repository) queuedListSize() (uint64, error) {
	return r.queuedList.GetSize()
}

func (r *Repository) popQueued() (thor.Address, *Validation, error) {
	head, err := r.queuedList.GetHead()
	if err != nil {
		return thor.Address{}, nil, errors.New("no head present")
	}

	if head.IsZero() {
		return thor.Address{}, nil, errors.New("list is empty")
	}

	entry, err := r.getValidation(head)
	if err != nil {
		return thor.Address{}, nil, err
	}
	if entry == nil {
		return thor.Address{}, nil, errors.New("entry is empty")
	}

	// otherwise, remove and return
	val, err := r.queuedList.Remove(head, entry)
	if err != nil {
		return thor.Address{}, nil, err
	}
	return head, val, nil
}

// removeQueued removes the entry from the queued list and persists the entry
func (r *Repository) removeQueued(address thor.Address, entry *Validation) error {
	_, err := r.queuedList.Remove(address, entry)
	return err
}

// addActive adds the entry to the active list and persists the entry
func (r *Repository) addActive(address thor.Address, newEntry *Validation) error {
	return r.activeList.Add(address, newEntry)
}

// removeActive removes the entry from the active list and persists the entry
func (r *Repository) removeActive(address thor.Address, entry *Validation) error {
	_, err := r.activeList.Remove(address, entry)
	return err
}

func (r *Repository) iterateActive(callback func(thor.Address, *Validation) error) error {
	return r.activeList.Iterate(callback)
}

func (r *Repository) iterateRenewalList(callbacks ...func(thor.Address, *Validation) error) error {
	return r.renewalList.Iterate(func(address thor.Address) error {
		entry, err := r.getValidation(address)
		if err != nil {
			return err
		}
		if entry == nil {
			return errors.New("entry is empty")
		}
		for _, callback := range callbacks {
			if err = callback(address, entry); err != nil {
				return err
			}
		}
		return nil
	})
}

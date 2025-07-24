// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"errors"
	"math/big"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

// linkedList is a doubly linked list implementation for validations, providing convenient operations for adding,
// removing, and popping validations.
// It allows us to maintain to linked list for both the queued validations and the active validations
type linkedList struct {
	head    *solidity.Bytes32
	tail    *solidity.Bytes32
	count   *solidity.Uint256
	storage *storage
}

func newLinkedList(
	storage *storage,
	headPos thor.Bytes32,
	tailPos thor.Bytes32,
	countPos thor.Bytes32,
) *linkedList {
	return &linkedList{
		head:    solidity.NewBytes32(storage.context, headPos),
		tail:    solidity.NewBytes32(storage.context, tailPos),
		count:   solidity.NewUint256(storage.context, countPos),
		storage: storage,
	}
}

// Pop removes the head of the linked list, sets the new head, and returns the removed head
func (l *linkedList) Pop() (thor.Bytes32, *Validation, error) {
	oldHeadID, err := l.head.Get()
	if err != nil {
		return thor.Bytes32{}, nil, err
	}
	if oldHeadID.IsZero() {
		return thor.Bytes32{}, nil, errors.New("no head present")
	}

	oldHead, err := l.storage.GetValidation(oldHeadID)
	if err != nil {
		return thor.Bytes32{}, nil, err
	}

	if _, err := l.Remove(oldHeadID, oldHead); err != nil {
		return thor.Bytes32{}, nil, err
	}

	return oldHeadID, oldHead, nil
}

// Remove removes a validator from the linked list
func (l *linkedList) Remove(id thor.Bytes32, validator *Validation) (removed bool, err error) {
	defer func() {
		if err == nil && removed {
			if subErr := l.count.Sub(big.NewInt(1)); subErr != nil {
				err = subErr
			}
		}
	}()

	prev := validator.Prev
	next := validator.Next

	// verify the entry exists in the linked list
	validatorEntry, err := l.storage.GetValidation(id)
	if err != nil {
		return false, err
	}
	if validatorEntry.IsEmpty() {
		return false, nil
	}

	if prev == nil || prev.IsZero() {
		l.head.Set(next, false)
	} else {
		prevEntry, err := l.storage.GetValidation(*prev)
		if err != nil {
			return false, err
		}
		prevEntry.Next = next
		if err := l.storage.SetValidation(*prev, prevEntry, false); err != nil {
			return false, err
		}
	}

	if next == nil || next.IsZero() {
		l.tail.Set(prev, false)
	} else {
		nextEntry, err := l.storage.GetValidation(*next)
		if err != nil {
			return false, err
		}
		nextEntry.Prev = prev
		if err := l.storage.SetValidation(*next, nextEntry, false); err != nil {
			return false, err
		}
	}

	// Clear references in the popped validator
	validator.Next = nil
	validator.Prev = nil

	return true, l.storage.SetValidation(id, validator, false)
}

// Add adds a new validator to the tail of the linked list
func (l *linkedList) Add(newTail thor.Bytes32, validation *Validation) (added bool, err error) {
	defer func() {
		if err == nil && added {
			if addErr := l.count.Add(big.NewInt(1)); addErr != nil {
				err = addErr
			}
		}
	}()

	// Clear any previous references in the new validator
	validation.Next = nil
	validation.Prev = nil

	oldTailID, err := l.tail.Get()
	if err != nil {
		return false, err
	}
	if oldTailID.IsZero() {
		// list is currently empty, set this entry to head & tail
		l.head.Set(&newTail, false)
		l.tail.Set(&newTail, false)
		return true, l.storage.SetValidation(newTail, validation, false)
	}

	oldTail, err := l.storage.GetValidation(oldTailID)
	if err != nil {
		return false, err
	}
	oldTail.Next = &newTail
	validation.Prev = &oldTailID

	if err := l.storage.SetValidation(oldTailID, oldTail, false); err != nil {
		return false, err
	}
	if err := l.storage.SetValidation(newTail, validation, false); err != nil {
		return false, err
	}

	l.tail.Set(&newTail, false)

	return true, nil
}

// Peek returns the head of the linked list
func (l *linkedList) Peek() (*Validation, error) {
	head, err := l.head.Get()
	if err != nil {
		return nil, err
	}
	return l.storage.GetValidation(head)
}

// Len returns the length of the linked list
func (l *linkedList) Len() (*big.Int, error) {
	return l.count.Get()
}

// Iter iterates through the linked list and calls the callback function for each entry
func (l *linkedList) Iter(callback func(thor.Bytes32, *Validation) error) error {
	ptr, err := l.head.Get()
	if err != nil {
		return err
	}
	for !ptr.IsZero() {
		entry, err := l.storage.GetValidation(ptr)
		if err != nil {
			return err
		}
		if entry.IsEmpty() {
			break
		}

		if err := callback(ptr, entry); err != nil {
			return err
		}

		if entry.Next == nil || entry.Next.IsZero() {
			break
		}
		ptr = *entry.Next
	}
	return nil
}

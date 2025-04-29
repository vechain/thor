// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

// linkedList is a doubly linked list implementation for validators, providing convenient operations for adding,
// removing, and popping validators.
// It allows us to maintain to linked list for both the queued validators and the active validators
type linkedList struct {
	head    *solidity.Bytes32
	tail    *solidity.Bytes32
	storage *storage
}

func newLinkedList(
	storage *storage,
	headPos thor.Bytes32,
	tailPos thor.Bytes32,
) *linkedList {
	return &linkedList{
		head:    solidity.NewBytes32(storage.Address(), storage.State(), headPos),
		tail:    solidity.NewBytes32(storage.Address(), storage.State(), tailPos),
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

	oldHead, err := l.storage.GetValidator(oldHeadID)
	if err != nil {
		return thor.Bytes32{}, nil, err
	}

	if oldHead.Next == nil || oldHead.Next.IsZero() {
		// This was the only element in the list
		l.head.Set(nil)
		l.tail.Set(nil)
	} else {
		// Set the new head
		newHeadID := oldHead.Next
		newHead, err := l.storage.GetValidator(*newHeadID)
		if err != nil {
			return thor.Bytes32{}, nil, err
		}
		newHead.Prev = nil

		if err := l.storage.SetValidator(*newHeadID, newHead); err != nil {
			return thor.Bytes32{}, nil, err
		}

		l.head.Set(newHeadID)
	}

	// Clear references in the removed validator
	oldHead.Next = nil
	oldHead.Prev = nil

	return oldHeadID, oldHead, nil
}

// Remove removes a validator from the linked list
func (l *linkedList) Remove(id thor.Bytes32, validator *Validation) error {
	prev := validator.Prev
	next := validator.Next

	// verify the entry exists in the linked list
	validatorEntry, err := l.storage.GetValidator(id)
	if err != nil {
		return err
	}
	if validatorEntry.IsEmpty() {
		return nil
	}

	if prev == nil || prev.IsZero() {
		l.head.Set(next)
	} else {
		prevEntry, err := l.storage.GetValidator(*prev)
		if err != nil {
			return err
		}
		prevEntry.Next = next
		if err := l.storage.SetValidator(*prev, prevEntry); err != nil {
			return err
		}
	}

	if next == nil || next.IsZero() {
		l.tail.Set(prev)
	} else {
		nextEntry, err := l.storage.GetValidator(*next)
		if err != nil {
			return err
		}
		nextEntry.Prev = prev
		if err := l.storage.SetValidator(*next, nextEntry); err != nil {
			return err
		}
	}

	// Clear references in the popped validator
	validator.Next = nil
	validator.Prev = nil

	return l.storage.SetValidator(id, validator)
}

// Add adds a new validator to the tail of the linked list
func (l *linkedList) Add(newTail thor.Bytes32, validation *Validation) error {
	// Clear any previous references in the new validator
	validation.Next = nil
	validation.Prev = nil

	oldTailID, err := l.tail.Get()
	if err != nil {
		return err
	}
	if oldTailID.IsZero() {
		// list is currently empty, set this entry to head & tail
		l.head.Set(&newTail)
		l.tail.Set(&newTail)
		return l.storage.SetValidator(newTail, validation)
	}

	oldTail, err := l.storage.GetValidator(oldTailID)
	if err != nil {
		return err
	}
	oldTail.Next = &newTail
	validation.Prev = &oldTailID

	if err := l.storage.SetValidator(oldTailID, oldTail); err != nil {
		return err
	}
	if err := l.storage.SetValidator(newTail, validation); err != nil {
		return err
	}

	l.tail.Set(&newTail)

	return nil
}

// Peek returns the head of the linked list
func (l *linkedList) Peek() (*Validation, error) {
	head, err := l.head.Get()
	if err != nil {
		return nil, err
	}
	return l.storage.GetValidator(head)
}

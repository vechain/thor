// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"github.com/vechain/thor/v2/thor"
)

// orderedLinkedList is a doubly linked list implementation for validations, providing convenient operations for adding,
// removing, and popping validations.
// It allows us to maintain to linked list for queued validations based on their weight
type orderedLinkedList struct {
	linkedList *linkedList
	storage    *storage
}

func newOrderedLinkedList(
	storage *storage,
	headPos thor.Bytes32,
	tailPos thor.Bytes32,
) *orderedLinkedList {
	return &orderedLinkedList{
		linkedList: newLinkedList(storage, headPos, tailPos),
		storage:    storage,
	}
}

// Pop removes the head of the ordered linked list, sets the new head, and returns the removed head
func (l *orderedLinkedList) Pop() (thor.Bytes32, *Validation, error) {
	return l.linkedList.Pop()
}

// Remove removes a validator from the linked list
func (l *orderedLinkedList) Remove(id thor.Bytes32, validator *Validation) error {
	return l.linkedList.Remove(id, validator)
}

// Add adds a new validator to appropriate position in the ordered linked list
func (l *orderedLinkedList) Add(id thor.Bytes32, newValidator *Validation) error {
	// Clear any previous references in the new validator
	newValidator.Next = nil
	newValidator.Prev = nil

	// Get the current head
	headID, err := l.linkedList.head.Get()
	if err != nil {
		return err
	}

	// If list is empty add to head and tail
	if headID.IsZero() {
		return l.linkedList.Add(id, newValidator)
	}

	head, err := l.storage.GetValidator(headID)
	if err != nil {
		return err
	}

	// New validator should be the new head if it has strictly higher weight
	if newValidator.PendingLocked.Cmp(head.PendingLocked) > 0 {
		newValidator.Next = &headID
		head.Prev = &id

		if err := l.storage.SetValidator(headID, head); err != nil {
			return err
		}
		if err := l.storage.SetValidator(id, newValidator); err != nil {
			return err
		}

		l.linkedList.head.Set(&id)
		return nil
	}

	// Traverse the list to find the appropriate position
	currentID := headID
	for {
		current, err := l.storage.GetValidator(currentID)
		if err != nil {
			return err
		}

		// If we've reached the end of the list, insert at tail
		if current.Next == nil || current.Next.IsZero() {
			current.Next = &id
			newValidator.Prev = &currentID
			if err := l.storage.SetValidator(currentID, current); err != nil {
				return err
			}
			if err := l.storage.SetValidator(id, newValidator); err != nil {
				return err
			}
			l.linkedList.tail.Set(&id)
			return nil
		}

		// Get the next validator to compare weights
		nextID := *current.Next
		next, err := l.storage.GetValidator(nextID)
		if err != nil {
			return err
		}

		// If new validator's weight is greater than next, insert here
		if newValidator.PendingLocked.Cmp(next.PendingLocked) > 0 {
			newValidator.Next = &nextID
			newValidator.Prev = &currentID
			current.Next = &id
			next.Prev = &id

			if err := l.storage.SetValidator(currentID, current); err != nil {
				return err
			}
			if err := l.storage.SetValidator(nextID, next); err != nil {
				return err
			}
			if err := l.storage.SetValidator(id, newValidator); err != nil {
				return err
			}
			return nil
		}

		// Move to next validator
		currentID = nextID
	}
}

// Peek returns the head of the linked list
func (l *orderedLinkedList) Peek() (*Validation, error) {
	return l.linkedList.Peek()
}

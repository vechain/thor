// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// orderedLinkedList is a doubly linked list implementation for validators, providing convenient operations for adding,
// removing, and popping validators.
// It allows us to maintain to linked list for queued validators based on their weight
type orderedLinkedList struct {
	linkedList *linkedList
}

func newOrderedLinkedList(
	addr thor.Address,
	state *state.State,
	validators *solidity.Mapping[thor.Address, *Validator],
	headPos thor.Bytes32,
	tailPos thor.Bytes32,
) *orderedLinkedList {
	return &orderedLinkedList{
		linkedList: newLinkedList(addr, state, validators, headPos, tailPos),
	}
}

// Pop removes the head of the ordered linked list, sets the new head, and returns the removed head
func (l *orderedLinkedList) Pop() (thor.Address, *Validator, error) {
	return l.linkedList.Pop()
}

// Remove removes a validator from the linked list
func (l *orderedLinkedList) Remove(addr thor.Address, validator *Validator) error {
	return l.linkedList.Remove(addr, validator)
}

// Add adds a new validator to appropriate position in the ordered linked list
func (l *orderedLinkedList) Add(address thor.Address, newValidator *Validator) error {
	// Clear any previous references in the new validator
	newValidator.Next = nil
	newValidator.Prev = nil

	// Get the current head
	headAddr, err := l.linkedList.head.Get()
	if err != nil {
		return err
	}

	// If list is empty add to head and tail
	if headAddr.IsZero() {
		l.linkedList.head.Set(&address)
		l.linkedList.tail.Set(&address)
		return l.linkedList.Add(address, newValidator)
	}

	head, err := l.linkedList.validators.Get(headAddr)
	if err != nil {
		return err
	}

	// New validator should be the new head if it has strictly higher weight
	if newValidator.Weight.Cmp(head.Weight) > 0 {
		newValidator.Next = &headAddr
		head.Prev = &address

		if err := l.linkedList.validators.Set(headAddr, head); err != nil {
			return err
		}
		if err := l.linkedList.validators.Set(address, newValidator); err != nil {
			return err
		}

		l.linkedList.head.Set(&address)
		return nil
	}

	// Traverse the list to find the appropriate position
	currentAddr := headAddr
	for {
		current, err := l.linkedList.validators.Get(currentAddr)
		if err != nil {
			return err
		}

		// If we've reached the end of the list, insert at tail
		if current.Next == nil || current.Next.IsZero() {
			current.Next = &address
			newValidator.Prev = &currentAddr
			if err := l.linkedList.validators.Set(currentAddr, current); err != nil {
				return err
			}
			if err := l.linkedList.validators.Set(address, newValidator); err != nil {
				return err
			}
			l.linkedList.tail.Set(&address)
			return nil
		}

		// Get the next validator to compare weights
		nextAddr := *current.Next
		next, err := l.linkedList.validators.Get(nextAddr)
		if err != nil {
			return err
		}

		// If new validator's weight is greater than next, insert here
		if newValidator.Weight.Cmp(next.Weight) > 0 {
			newValidator.Next = &nextAddr
			newValidator.Prev = &currentAddr
			current.Next = &address
			next.Prev = &address

			if err := l.linkedList.validators.Set(currentAddr, current); err != nil {
				return err
			}
			if err := l.linkedList.validators.Set(nextAddr, next); err != nil {
				return err
			}
			if err := l.linkedList.validators.Set(address, newValidator); err != nil {
				return err
			}
			return nil
		}

		// Move to next validator
		currentAddr = nextAddr
	}
}

// Peek returns the head of the linked list
func (l *orderedLinkedList) Peek() (*Validator, error) {
	return l.linkedList.Peek()
}

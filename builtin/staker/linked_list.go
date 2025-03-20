// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// linkedList is a doubly linked list implementation for validators, providing convenient operations for adding,
// removing, and popping validators.
// It allows us to maintain to linked list for both the queued validators and the active validators
type linkedList struct {
	addr       thor.Address
	head       *solidity.Address
	tail       *solidity.Address
	validators *solidity.Mapping[thor.Address, *Validator]
}

func newLinkedList(
	addr thor.Address,
	state *state.State,
	validators *solidity.Mapping[thor.Address, *Validator],
	headPos thor.Bytes32,
	tailPos thor.Bytes32,
) *linkedList {
	return &linkedList{
		addr:       addr,
		head:       solidity.NewAddress(addr, state, headPos),
		tail:       solidity.NewAddress(addr, state, tailPos),
		validators: validators,
	}
}

// Pop removes the head of the linked list, sets the new head, and returns the removed head
func (l *linkedList) Pop() (thor.Address, *Validator, error) {
	oldHeadAddr, err := l.head.Get()
	if err != nil {
		return thor.Address{}, nil, err
	}
	if oldHeadAddr.IsZero() {
		return thor.Address{}, nil, errors.New("no head present")
	}

	oldHead, err := l.validators.Get(oldHeadAddr)
	if err != nil {
		return thor.Address{}, nil, err
	}

	if oldHead.Next == nil || oldHead.Next.IsZero() {
		// This was the only element in the list
		l.head.Set(nil)
		l.tail.Set(nil)
	} else {
		// Set the new head
		newHeadAddr := oldHead.Next
		newHead, err := l.validators.Get(*newHeadAddr)
		if err != nil {
			return thor.Address{}, nil, err
		}
		newHead.Prev = nil

		if err := l.validators.Set(*newHeadAddr, newHead); err != nil {
			return thor.Address{}, nil, err
		}

		l.head.Set(newHeadAddr)
	}

	// Clear references in the removed validator
	oldHead.Next = nil
	oldHead.Prev = nil

	return oldHeadAddr, oldHead, nil
}

// Remove removes a validator from the linked list
func (l *linkedList) Remove(addr thor.Address, validator *Validator) error {
	prev := validator.Prev
	next := validator.Next

	if prev == nil || prev.IsZero() {
		l.head.Set(next)
	} else {
		prevEntry, err := l.validators.Get(*prev)
		if err != nil {
			return err
		}
		prevEntry.Next = next
		if err := l.validators.Set(*prev, prevEntry); err != nil {
			return err
		}
	}

	if next == nil || next.IsZero() {
		l.tail.Set(prev)
	} else {
		nextEntry, err := l.validators.Get(*next)
		if err != nil {
			return err
		}
		nextEntry.Prev = prev
		if err := l.validators.Set(*next, nextEntry); err != nil {
			return err
		}
	}

	// Clear references in the popped validator
	validator.Next = nil
	validator.Prev = nil

	return l.validators.Set(addr, validator)
}

// Add adds a new validator to the tail of the linked list
func (l *linkedList) Add(newTailAddr thor.Address, newTail *Validator) error {
	// Clear any previous references in the new validator
	newTail.Next = nil
	newTail.Prev = nil

	oldTailAddr, err := l.tail.Get()
	if err != nil {
		return err
	}
	if oldTailAddr.IsZero() {
		// list is currently empty, set this entry to head & tail
		l.head.Set(&newTailAddr)
		l.tail.Set(&newTailAddr)
		return l.validators.Set(newTailAddr, newTail)
	}

	oldTail, err := l.validators.Get(oldTailAddr)
	if err != nil {
		return err
	}
	oldTail.Next = &newTailAddr
	newTail.Prev = &oldTailAddr

	if err := l.validators.Set(oldTailAddr, oldTail); err != nil {
		return err
	}
	if err := l.validators.Set(newTailAddr, newTail); err != nil {
		return err
	}

	l.tail.Set(&newTailAddr)

	return nil
}

// Peek returns the head of the linked list
func (l *linkedList) Peek() (*Validator, error) {
	head, err := l.head.Get()
	if err != nil {
		return nil, err
	}
	return l.validators.Get(head)
}

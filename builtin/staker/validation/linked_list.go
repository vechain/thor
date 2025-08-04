// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"errors"
	"math/big"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

// LinkedList is a doubly linked list implementation for validations, providing convenient operations for adding,
// removing, and popping validations.
// It allows us to maintain to linked list for both the queued validations and the active validations
type LinkedList struct {
	head  *solidity.Address
	tail  *solidity.Address
	count *solidity.Uint256
	repo  *Repository
}

func NewLinkedList(sctx *solidity.Context, repo *Repository, headPos thor.Bytes32, tailPos thor.Bytes32, countPos thor.Bytes32) *LinkedList {
	return &LinkedList{
		head:  solidity.NewAddress(sctx, headPos),
		tail:  solidity.NewAddress(sctx, tailPos),
		count: solidity.NewUint256(sctx, countPos),
		repo:  repo,
	}
}

// Pop removes the head of the linked list, sets the new head, and returns the removed head
func (l *LinkedList) Pop() (thor.Address, *Validation, error) {
	oldHeadID, err := l.head.Get()
	if err != nil {
		return thor.Address{}, nil, errors.New("no head present")
	}

	oldHead, err := l.repo.GetValidation(oldHeadID)
	if err != nil {
		return thor.Address{}, nil, err
	}

	if _, err := l.Remove(oldHeadID, oldHead); err != nil {
		return thor.Address{}, nil, err
	}

	return oldHeadID, oldHead, nil
}

// Remove removes a validator from the linked list
func (l *LinkedList) Remove(validator thor.Address, validation *Validation) (removed bool, err error) {
	defer func() {
		if err == nil && removed {
			if subErr := l.count.Sub(big.NewInt(1)); subErr != nil {
				err = subErr
			}
		}
	}()

	prev := validation.Prev
	next := validation.Next

	// verify the entry exists in the linked list
	validatorEntry, err := l.repo.GetValidation(validator)
	if err != nil {
		return false, err
	}
	if validatorEntry.IsEmpty() {
		return false, nil
	}

	if prev == nil || prev.IsZero() {
		l.head.Set(next, false)
	} else {
		prevEntry, err := l.repo.GetValidation(*prev)
		if err != nil {
			return false, err
		}
		prevEntry.Next = next
		// TODO keep the LL separated from the Validation storage ops
		if err := l.repo.SetValidation(*prev, prevEntry, false); err != nil {
			return false, err
		}
	}

	if next == nil || next.IsZero() {
		l.tail.Set(prev, false)
	} else {
		nextEntry, err := l.repo.GetValidation(*next)
		if err != nil {
			return false, err
		}
		nextEntry.Prev = prev
		// TODO keep the LL separated from the Validation storage ops
		if err := l.repo.SetValidation(*next, nextEntry, false); err != nil {
			return false, err
		}
	}

	// Clear references in the popped validator
	validation.Next = nil
	validation.Prev = nil

	return true, l.repo.SetValidation(validator, validation, false)
}

// Add adds a new validator to the tail of the linked list
func (l *LinkedList) Add(newTail thor.Address, validation *Validation) (added bool, err error) {
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
		// TODO keep the LL separated from the Validation storage ops
		return true, l.repo.SetValidation(newTail, validation, false)
	}

	oldTail, err := l.repo.GetValidation(oldTailID)
	if err != nil {
		return false, err
	}
	oldTail.Next = &newTail
	validation.Prev = &oldTailID

	// TODO keep the LL separated from the Validation storage ops
	if err := l.repo.SetValidation(oldTailID, oldTail, false); err != nil {
		return false, err
	}
	if err := l.repo.SetValidation(newTail, validation, false); err != nil {
		return false, err
	}

	l.tail.Set(&newTail, false)

	return true, nil
}

// Peek returns the head of the linked list
func (l *LinkedList) Peek() (*Validation, error) {
	head, err := l.head.Get()
	if err != nil {
		return nil, err
	}
	return l.repo.GetValidation(head)
}

// Len returns the length of the linked list
func (l *LinkedList) Len() (*big.Int, error) {
	return l.count.Get()
}

// Iter iterates through the linked list and calls the callback function for each entry
func (l *LinkedList) Iter(callback func(thor.Address, *Validation) error) error {
	ptr, err := l.head.Get()
	if err != nil {
		return err
	}
	for !ptr.IsZero() {
		entry, err := l.repo.GetValidation(ptr)
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

func (l *LinkedList) Count() (*big.Int, error) {
	return l.count.Get()
}

func (l *LinkedList) Head() (thor.Address, error) {
	return l.head.Get()
}

// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package linkedlist

import (
	"math/big"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

type LinkedList struct {
	head  *solidity.Address
	tail  *solidity.Address
	count *solidity.Uint256
	next  *solidity.Mapping[thor.Address, thor.Address]
	prev  *solidity.Mapping[thor.Address, thor.Address]
}

func NewLinkedList(sctx *solidity.Context, headPos, tailPos, countPos thor.Bytes32) *LinkedList {
	return &LinkedList{
		head:  solidity.NewAddress(sctx, headPos),
		tail:  solidity.NewAddress(sctx, tailPos),
		count: solidity.NewUint256(sctx, countPos),
		next:  solidity.NewMapping[thor.Address, thor.Address](sctx, headPos),
		prev:  solidity.NewMapping[thor.Address, thor.Address](sctx, tailPos),
	}
}

// Add adds a new address to the tail of the linked list
func (l *LinkedList) Add(address thor.Address) error {
	oldTail, err := l.tail.Get()
	if err != nil {
		return err
	}

	if oldTail.IsZero() {
		// list is currently empty, set this entry to head & tail
		l.head.Set(&address, false)
		l.tail.Set(&address, false)
		return l.count.Add(big.NewInt(1))
	}

	// Update old tail's next pointer
	if err := l.next.Set(oldTail, address, false); err != nil {
		return err
	}

	// Set new tail's prev pointer
	if err := l.prev.Set(address, oldTail, false); err != nil {
		return err
	}

	// Update tail pointer
	l.tail.Set(&address, false)

	return l.count.Add(big.NewInt(1))
}

// Remove removes an address from the linked list
func (l *LinkedList) Remove(address thor.Address) error {
	if address.IsZero() {
		return nil
	}

	prev, err := l.prev.Get(address)
	if err != nil {
		return err
	}

	next, err := l.next.Get(address)
	if err != nil {
		return err
	}

	// If address is not in the list (no prev and not head)
	if prev.IsZero() && !l.isHead(address) {
		return nil // not in list
	}

	// Update previous node's next pointer
	if !prev.IsZero() {
		if err := l.next.Set(prev, next, false); err != nil {
			return err
		}
	} else {
		// This is the head, update head pointer
		l.head.Set(&next, false)
	}

	// Update next node's prev pointer
	if !next.IsZero() {
		if err := l.prev.Set(next, prev, false); err != nil {
			return err
		}
	} else {
		// This is the tail, update tail pointer
		l.tail.Set(&prev, false)
	}

	// Clear the removed node's pointers
	if err = l.next.Set(address, thor.Address{}, false); err != nil {
		return err
	}
	if err = l.prev.Set(address, thor.Address{}, false); err != nil {
		return err
	}

	return l.count.Sub(big.NewInt(1))
}

// Pop removes the head of the linked list and returns the removed address
func (l *LinkedList) Pop() (thor.Address, error) {
	head, err := l.head.Get()
	if err != nil {
		return thor.Address{}, errors.New("no head present")
	}

	if head.IsZero() {
		return thor.Address{}, errors.New("list is empty")
	}

	// otherwise, remove and return
	if err := l.Remove(head); err != nil {
		return thor.Address{}, err
	}
	return head, nil
}

// Peek returns the head address without removing it
func (l *LinkedList) Peek() (thor.Address, error) {
	return l.head.Get()
}

// Len returns the length of the linked list
func (l *LinkedList) Len() (*big.Int, error) {
	return l.count.Get()
}

// Iter iterates through the linked list and calls the callback function for each address
func (l *LinkedList) Iter(callback func(thor.Address) error) error {
	ptr, err := l.head.Get()
	if err != nil {
		return err
	}

	for !ptr.IsZero() {
		if err := callback(ptr); err != nil {
			return err
		}

		next, err := l.next.Get(ptr)
		if err != nil {
			return err
		}

		if next.IsZero() {
			break
		}
		ptr = next
	}

	return nil
}

// Next returns the next address in the list
func (l *LinkedList) Next(address thor.Address) (thor.Address, error) {
	return l.next.Get(address)
}

// isHead checks if the given address is the head of the list
func (l *LinkedList) isHead(address thor.Address) bool {
	head, err := l.head.Get()
	if err != nil {
		return false
	}
	return head == address
}

func (l *LinkedList) Head() (thor.Address, error) {
	return l.head.Get()
}

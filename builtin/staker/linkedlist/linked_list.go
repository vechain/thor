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

	lru "github.com/hashicorp/golang-lru"
)

type LinkedList struct {
	head  *solidity.Address
	tail  *solidity.Address
	count *solidity.Uint256
	next  *solidity.Mapping[thor.Address, thor.Address]
	prev  *solidity.Mapping[thor.Address, thor.Address]

	cache *lru.Cache
}

type cacheEntry struct {
	headAddress thor.Address
	nextMap     map[thor.Address]thor.Address
}

// NewLinkedList creates a new linked list with persistent storage mappings for staker management
func NewLinkedList(sctx *solidity.Context, headPos, tailPos, countPos thor.Bytes32) *LinkedList {
	cache, _ := lru.New(16)

	return &LinkedList{
		head:  solidity.NewAddress(sctx, headPos),
		tail:  solidity.NewAddress(sctx, tailPos),
		count: solidity.NewUint256(sctx, countPos),
		next:  solidity.NewMapping[thor.Address, thor.Address](sctx, headPos),
		prev:  solidity.NewMapping[thor.Address, thor.Address](sctx, tailPos),
		cache: cache,
	}
}

func (l *LinkedList) loadCache(headAddr *solidity.Address) (*cacheEntry, error) {
	if cached, ok := l.cache.Get(headAddr); ok {
		return cached.(*cacheEntry), nil
	}

	head, err := l.head.Get()
	if err != nil {
		return nil, err
	}

	nextMap := make(map[thor.Address]thor.Address)
	ptr := head
	for !ptr.IsZero() {
		next, err := l.next.Get(ptr)
		if err != nil {
			return nil, err
		}
		nextMap[ptr] = next
		ptr = next
	}

	entry := &cacheEntry{
		headAddress: head,
		nextMap:     nextMap,
	}

	l.cache.Add(entry.headAddress, entry)
	return entry, nil
}

// Iter traverses the list in FIFO order, calling callback for each address until completion or error
func (l *LinkedList) Iter(callbacks ...func(thor.Address) error) error {
	entry, err := l.loadCache(l.head)
	if err != nil {
		return err
	}

	ptr := entry.headAddress
	for !ptr.IsZero() {
		for _, callback := range callbacks {
			if err := callback(ptr); err != nil {
				return err
			}
		}
		ptr = entry.nextMap[ptr]
	}

	return nil
}

// Add appends an address to the end of the list, maintaining FIFO order for staker processing
func (l *LinkedList) Add(address thor.Address) error {
	oldTail, err := l.tail.Get()
	if err != nil {
		return err
	}

	if oldTail.IsZero() {
		// the list is currently empty, set this entry to head & tail
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

	if err := l.count.Add(big.NewInt(1)); err != nil {
		return err
	}

	l.cache.Purge()
	return err
}

// Remove extracts an address from anywhere in the list, reconnecting adjacent nodes and clearing removed node's pointers
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
	if prev.IsZero() {
		isHead, err := l.isHead(address)
		if err != nil {
			return err
		}
		if !isHead {
			return nil // not in list
		}
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

	if err := l.count.Sub(big.NewInt(1)); err != nil {
		return err
	}

	l.cache.Purge()
	return err
}

// Pop removes and returns the oldest entry (head) for FIFO processing order
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

// Peek returns the next address to be processed without removing it from the queue
func (l *LinkedList) Peek() (thor.Address, error) {
	return l.head.Get()
}

// Len returns the current number of addresses in the staker queue
func (l *LinkedList) Len() (*big.Int, error) {
	return l.count.Get()
}

// Next returns the successor address in the list, or zero address if at the end
func (l *LinkedList) Next(address thor.Address) (thor.Address, error) {
	return l.next.Get(address)
}

// isHead checks if the given address is the head of the list
func (l *LinkedList) isHead(address thor.Address) (bool, error) {
	head, err := l.head.Get()
	if err != nil {
		return false, err
	}
	return head == address, nil
}

// Head returns the oldest address in the queue (next to be processed)
func (l *LinkedList) Head() (thor.Address, error) {
	return l.head.Get()
}

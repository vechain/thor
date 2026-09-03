// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

// renewalList is a doubly linked list implementation to store the toUpdate list for
// validation service, which is self sufficient, no outside dependency.
type renewalList struct {
	head *solidity.Raw[thor.Address]
	tail *solidity.Raw[thor.Address]
	prev *solidity.Mapping[thor.Address, thor.Address]
	next *solidity.Mapping[thor.Address, thor.Address]
}

func newRenewalList(sctx *solidity.Context, headKey, tailKey, prevKey, nextKey thor.Bytes32) *renewalList {
	return &renewalList{
		head: solidity.NewRaw[thor.Address](sctx, headKey),
		tail: solidity.NewRaw[thor.Address](sctx, tailKey),
		prev: solidity.NewMapping[thor.Address, thor.Address](sctx, prevKey),
		next: solidity.NewMapping[thor.Address, thor.Address](sctx, nextKey),
	}
}

func (r *renewalList) contains(key thor.Address) (bool, error) {
	if key.IsZero() {
		return false, nil
	}

	val, err := r.next.Get(key)
	if err != nil {
		return false, err
	}

	// has next and not zero
	if !val.IsZero() {
		return true, nil
	}

	// check if key is the tail
	tail, err := r.tail.Get()
	if err != nil {
		return false, err
	}
	return tail == key, nil
}

// Add adds a new entry to the list，this is a idempotent operation.
func (r *renewalList) Add(newKey thor.Address) error {
	if newKey.IsZero() {
		return nil
	}

	has, err := r.contains(newKey)
	if err != nil {
		return err
	}
	// already in the list
	if has {
		return nil
	}

	prevTail, err := r.tail.Get()
	if err != nil {
		return err
	}

	if prevTail.IsZero() {
		// this list is empty, set head and tail to newKey
		if err := r.head.Upsert(newKey); err != nil {
			return err
		}
		if err := r.tail.Upsert(newKey); err != nil {
			return err
		}

		return nil
	}

	// Update prev tail's next pointer
	if err := r.next.Upsert(prevTail, newKey); err != nil {
		return err
	}
	// Update new key's prev pointer
	if err := r.prev.Upsert(newKey, prevTail); err != nil {
		return err
	}
	return r.tail.Upsert(newKey)
}

func (r *renewalList) Remove(toRemove thor.Address) error {
	if toRemove.IsZero() {
		return nil
	}

	has, err := r.contains(toRemove)
	if err != nil {
		return err
	}
	// not in the list
	if !has {
		return nil
	}

	prev, err := r.prev.Get(toRemove)
	if err != nil {
		return err
	}

	next, err := r.next.Get(toRemove)
	if err != nil {
		return err
	}

	if prev.IsZero() && next.IsZero() {
		// entry is not linked, check if it is the only element in the list
		head, err := r.head.Get()
		if err != nil {
			return err
		}

		// not the head, not the only element
		if head.IsZero() || head != toRemove {
			return nil
		}

		tail, err := r.tail.Get()
		if err != nil {
			return err
		}

		// not the tail, not the only element
		if tail.IsZero() || tail != toRemove {
			return nil
		}

		// last element, fall back to default behavior, will reset head and tail to zero
	}

	if prev.IsZero() {
		// entry is the head, update head to next
		// headKey is touched previously since entry is linked
		if err := r.head.Update(next); err != nil {
			return err
		}
	} else {
		// update prev's next pointer
		if err := r.next.Update(prev, next); err != nil {
			return err
		}
	}

	if next.IsZero() {
		// entry is the tail, update tail to prev
		// tailKey is touched previously since entry is linked
		if err := r.tail.Update(prev); err != nil {
			return err
		}
	} else {
		// update next's prev pointer
		if err := r.prev.Update(next, prev); err != nil {
			return err
		}
	}

	// clear removed entry's prev and next pointers
	if err := r.prev.Update(toRemove, thor.Address{}); err != nil {
		return err
	}
	return r.next.Update(toRemove, thor.Address{})
}

func (r *renewalList) Iterate(callbacks func(thor.Address) error) error {
	current, err := r.head.Get()
	if err != nil {
		return err
	}

	for !current.IsZero() {
		if err := callbacks(current); err != nil {
			return err
		}
		next, err := r.next.Get(current)
		if err != nil {
			return err
		}

		current = next
	}

	return nil
}

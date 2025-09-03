// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package validation

import (
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/thor"
)

type LinkedListEntry struct {
	Prev *thor.Address `rlp:"nil"`
	Next *thor.Address `rlp:"nil"`
}

func (l *LinkedListEntry) SetPrev(prev *thor.Address) {
	l.Prev = prev
}

func (l *LinkedListEntry) SetNext(next *thor.Address) {
	l.Next = next
}

func (l *LinkedListEntry) IsLinked() bool {
	return l.Prev != nil || l.Next != nil
}

type listStats struct {
	head    *solidity.Raw[*thor.Address]
	tail    *solidity.Raw[*thor.Address]
	size    *solidity.Raw[uint64]
	storage *Storage
}

func newListStats(sctx *solidity.Context, storage *Storage, headKey, tailKey, sizeKey thor.Bytes32) *listStats {
	return &listStats{
		head:    solidity.NewRaw[*thor.Address](sctx, headKey),
		tail:    solidity.NewRaw[*thor.Address](sctx, tailKey),
		size:    solidity.NewRaw[uint64](sctx, sizeKey),
		storage: storage,
	}
}

///
/// Public methods - receive and return values
///

func (l *listStats) GetHead() (thor.Address, error) {
	head, err := l.head.Get()
	if err != nil {
		return thor.Address{}, err
	}
	if head == nil {
		return thor.Address{}, nil
	}
	return *head, nil
}

func (l *listStats) GetTail() (thor.Address, error) {
	tail, err := l.tail.Get()
	if err != nil {
		return thor.Address{}, err
	}
	return *tail, nil
}

func (l *listStats) GetSize() (uint64, error) {
	return l.size.Get()
}

func (l *listStats) Remove(address thor.Address, entry *Validation) (*Validation, error) {
	if !entry.IsLinked() {
		// if entry is not linked, check if it is the last element in the list
		head, err := l.head.Get()
		if err != nil {
			return nil, err
		}
		if head == nil || head.String() != address.String() {
			// not the last element, return entry anyway
			return entry, nil
		}

		tail, err := l.head.Get()
		if err != nil {
			return nil, err
		}
		if tail == nil || tail.String() != address.String() {
			// not the last element, return entry anyway
			return entry, nil
		}

		// last element, set head and tail to nil
		if err := l.setHead(nil); err != nil {
			return nil, err
		}
		if err := l.setTail(nil); err != nil {
			return nil, err
		}
		// subtract size
		if err := l.subSize(); err != nil {
			return nil, err
		}

		// update the entry
		if err := l.storage.updateValidation(address, entry); err != nil {
			return nil, err
		}

		return entry, nil
	}

	if entry.Prev == nil {
		// entry is the head
		if err := l.setHead(entry.Next); err != nil {
			return nil, err
		}
	} else {
		prevEntry, err := l.storage.getValidation(*entry.Prev)
		if err != nil {
			return nil, err
		}
		if prevEntry == nil {
			return nil, errors.New("prev entry is empty")
		}

		prevEntry.SetNext(entry.Next)
		if err := l.storage.updateValidation(*entry.Prev, prevEntry); err != nil {
			return nil, err
		}
	}

	if entry.Next == nil {
		// entry is the tail
		if err := l.setTail(entry.Prev); err != nil {
			return nil, err
		}
	} else {
		nextEntry, err := l.storage.getValidation(*entry.Next)
		if err != nil {
			return nil, err
		}
		if nextEntry == nil {
			return nil, errors.New("next entry is empty")
		}

		nextEntry.SetPrev(entry.Prev)
		if err := l.storage.updateValidation(*entry.Next, nextEntry); err != nil {
			return nil, err
		}
	}

	// clear the entry pointers
	entry.SetPrev(nil)
	entry.SetNext(nil)

	// update list size
	if err := l.subSize(); err != nil {
		return nil, err
	}

	// update the entry
	if err := l.storage.updateValidation(address, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

func (l *listStats) Add(address thor.Address, newEntry *Validation) error {
	tail, err := l.tail.Get()
	if err != nil {
		return err
	}

	// set the new entry's prev to the tail
	newEntry.SetPrev(tail)
	// add new queued to the tail
	if err := l.setTail(&address); err != nil {
		return err
	}

	// list is empty
	if tail == nil {
		if err := l.setHead(&address); err != nil {
			return err
		}
	} else {
		tailEntry, err := l.storage.getValidation(*tail)
		if err != nil {
			return err
		}

		if tailEntry == nil {
			return errors.New("tail entry is empty")
		}

		// update link list pointers
		newEntry.SetPrev(tail)
		tailEntry.SetNext(&address)

		if err := l.storage.updateValidation(*tail, tailEntry); err != nil {
			return err
		}
	}

	// update list size
	if err := l.addSize(); err != nil {
		return err
	}

	// update or add new entry
	return l.storage.upsertValidation(address, newEntry)
}

///
/// Private Methods - use pointers
//

func (l *listStats) setHead(key *thor.Address) error {
	return l.head.Upsert(key)
}

func (l *listStats) setTail(key *thor.Address) error {
	return l.tail.Upsert(key)
}

func (l *listStats) addSize() error {
	size, err := l.size.Get()
	if err != nil {
		return err
	}
	return l.size.Upsert(size + 1)
}

func (l *listStats) subSize() error {
	size, err := l.size.Get()
	if err != nil {
		return err
	}
	if size == 0 {
		return errors.New("size is already 0")
	}

	// already touched by AddSize
	return l.size.Update(size - 1)
}

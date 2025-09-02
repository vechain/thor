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
	head *solidity.Raw[*thor.Address]
	tail *solidity.Raw[*thor.Address]
	size *solidity.Raw[uint64]
}

func newListStats(sctx *solidity.Context, headKey, tailKey, sizeKey thor.Bytes32) *listStats {
	return &listStats{
		head: solidity.NewRaw[*thor.Address](sctx, headKey),
		tail: solidity.NewRaw[*thor.Address](sctx, tailKey),
		size: solidity.NewRaw[uint64](sctx, sizeKey),
	}
}

func (l *listStats) GetHead() (*thor.Address, error) {
	return l.head.Get()
}

func (l *listStats) GetTail() (*thor.Address, error) {
	return l.tail.Get()
}

func (l *listStats) GetSize() (uint64, error) {
	return l.size.Get()
}

func (l *listStats) SetHead(key *thor.Address) error {
	return l.head.Upsert(key)
}

func (l *listStats) SetTail(key *thor.Address) error {
	return l.tail.Upsert(key)
}

func (l *listStats) AddSize() error {
	size, err := l.size.Get()
	if err != nil {
		return err
	}
	return l.size.Upsert(size + 1)
}

func (l *listStats) SubSize() error {
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

func removeFromList(repo *Repository, list *listStats, address thor.Address, entry *Validation) (*Validation, error) {
	if !entry.IsLinked() {
		// if entry is not linked, check if it is the last element in the list
		head, err := list.GetHead()
		if err != nil {
			return nil, err
		}
		if head == nil || *head != address {
			// not the last element, return entry anyway
			return entry, nil
		}

		tail, err := list.GetTail()
		if err != nil {
			return nil, err
		}
		if tail == nil || *tail != address {
			// not the last element, return entry anyway
			return entry, nil
		}

		// last element, set head and tail to nil
		if err := list.SetHead(nil); err != nil {
			return nil, err
		}
		if err := list.SetTail(nil); err != nil {
			return nil, err
		}
		// subtract size
		if err := list.SubSize(); err != nil {
			return nil, err
		}

		// update the entry
		if err := repo.updateValidation(address, entry); err != nil {
			return nil, err
		}

		return entry, nil
	}

	if entry.Prev == nil {
		// entry is the head
		if err := list.SetHead(entry.Next); err != nil {
			return nil, err
		}
	} else {
		prevEntry, err := repo.getValidation(*entry.Prev)
		if err != nil {
			return nil, err
		}
		if prevEntry.IsEmpty() {
			return nil, errors.New("prev entry is empty")
		}

		prevEntry.SetNext(entry.Next)
		if err := repo.updateValidation(*entry.Prev, prevEntry); err != nil {
			return nil, err
		}
	}

	if entry.Next == nil {
		// entry is the tail
		if err := list.SetTail(entry.Prev); err != nil {
			return nil, err
		}
	} else {
		nextEntry, err := repo.getValidation(*entry.Next)
		if err != nil {
			return nil, err
		}
		if nextEntry.IsEmpty() {
			return nil, errors.New("next entry is empty")
		}

		nextEntry.SetPrev(entry.Prev)
		if err := repo.updateValidation(*entry.Next, nextEntry); err != nil {
			return nil, err
		}
	}

	// clear the entry pointers
	entry.SetPrev(nil)
	entry.SetNext(nil)

	// update list size
	if err := list.SubSize(); err != nil {
		return nil, err
	}

	// update the entry
	if err := repo.updateValidation(address, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

func addToList(repo *Repository, list *listStats, address thor.Address, newEntry *Validation) error {
	tail, err := list.GetTail()
	if err != nil {
		return err
	}

	// set the new entry's prev to the tail
	newEntry.SetPrev(tail)
	// add new queued to the tail
	if err := list.SetTail(&address); err != nil {
		return err
	}

	// list is empty
	if tail == nil {
		if err := list.SetHead(&address); err != nil {
			return err
		}
	} else {
		tailEntry, err := repo.getValidation(*tail)
		if err != nil {
			return err
		}

		if tailEntry.IsEmpty() {
			return errors.New("tail entry is empty")
		}

		// update link list pointers
		newEntry.SetPrev(tail)
		tailEntry.SetNext(&address)

		if err := repo.updateValidation(*tail, tailEntry); err != nil {
			return err
		}
	}

	// update list size
	if err := list.AddSize(); err != nil {
		return err
	}

	// update or add new entry
	if list == repo.queuedList {
		return repo.validations.Set(address, *newEntry, true)
	}
	return repo.validations.Set(address, *newEntry, false)
}

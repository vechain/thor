// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	Sort "sort"
	"sync"

	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/thor"
)

type quota map[thor.Address]uint

func (q quota) inc(signer thor.Address) {
	if v, ok := q[signer]; ok {
		q[signer] = v + 1
	} else {
		q[signer] = 1
	}
}

func (q quota) dec(signer thor.Address) {
	if v, ok := q[signer]; ok {
		if v > 1 {
			q[signer] = v - 1
		} else {
			delete(q, signer)
		}
	}
}

type entry struct {
	lock    sync.Mutex
	dirty   bool
	all     *cache.RandCache
	pending txObjects
	sorted  bool
	quota   quota
}

func newEntry(size int) *entry {
	return &entry{
		all:   cache.NewRandCache(size),
		quota: make(quota),
	}
}

func (e *entry) find(id thor.Bytes32) *txObject {
	e.lock.Lock()
	defer e.lock.Unlock()

	if value, ok := e.all.Get(id); ok {
		if obj, ok := value.(*txObject); ok {
			return obj
		}
	}
	return nil
}

func (e *entry) size() int {
	e.lock.Lock()
	defer e.lock.Unlock()

	return e.all.Len()
}

func (e *entry) pick() {
	e.lock.Lock()
	defer e.lock.Unlock()

	if picked, ok := e.all.Pick().Value.(*txObject); ok {
		e.quota.dec(picked.signer)
		e.all.Remove(picked.tx.ID())
	}
}

func (e *entry) delete(id thor.Bytes32) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if value, ok := e.all.Get(id); ok {
		if obj, ok := value.(*txObject); ok {
			e.quota.dec(obj.signer)
			e.all.Remove(id)
			obj.deleted = true
		}
	}
}

func (e *entry) save(obj *txObject) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if _, ok := e.all.Get(obj.tx.ID()); !ok {
		e.quota.inc(obj.signer)
	}

	e.all.Set(obj.tx.ID(), obj)
	e.dirty = true

}

func (e *entry) dumpPending(sort bool) txObjects {
	e.lock.Lock()
	defer e.lock.Unlock()

	if e.dirty {
		return nil
	}

	if sort && !e.sorted {
		Sort.Slice(e.pending, func(i, j int) bool {
			return e.pending[i].overallGP.Cmp(e.pending[j].overallGP) > 0
		})
		e.sorted = true
	}

	size := len(e.pending)
	pending := make(txObjects, size, size)

	for i, obj := range e.pending {
		pending[i] = obj
	}

	return pending
}

func (e *entry) dumpAll() txObjects {
	e.lock.Lock()
	defer e.lock.Unlock()

	all := make(txObjects, 0, e.all.Len())
	e.all.ForEach(func(entry *cache.Entry) bool {
		if obj, ok := entry.Value.(*txObject); ok {
			all = append(all, obj)
			return true
		}
		return false
	})

	return all
}

func (e *entry) cachePending(pending txObjects) {
	e.lock.Lock()
	defer e.lock.Unlock()

	e.pending = pending
	e.sorted = false
	e.dirty = false
}

func (e *entry) isDirty() bool {
	e.lock.Lock()
	defer e.lock.Unlock()

	return e.dirty
}

func (e *entry) quotaBySinger(signer thor.Address) uint {
	e.lock.Lock()
	defer e.lock.Unlock()

	if v, ok := e.quota[signer]; ok {
		return v
	}
	return 0
}

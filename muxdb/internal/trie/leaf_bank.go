// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"encoding/binary"
	"math"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/kv"
)

// Leaf is the entity stored in leaf bank.
type Leaf struct {
	Value, Meta []byte
	CommitNum   uint32
}

// SaveLeaf is the function to save a leaf.
type SaveLeaf func(key, value, meta []byte) error

// leafBankSlot presents per-trie state and cached leaves.
type leafBankSlot struct {
	maxCommitNum uint32     // max recorded commit number, for empty leaf assertion
	cache        *lru.Cache // cached leaves
}

// LeafBank records accumulated trie leaves to help accelerate trie leaf access
// according to VIP-212.
type LeafBank struct {
	store   kv.Store
	newSlot func() *leafBankSlot
	slots   *lru.ARCCache
}

// NewLeafBank creates a new LeafBank instance.
// The slotCap indicates the capacity of cached per-trie slots.
// The perSlotLeavesCap indicates capacity of leaves cache of a trie slot.
func NewLeafBank(store kv.Store, slotCap int, perSlotLeavesCap int) *LeafBank {
	b := &LeafBank{
		store: kv.Bucket(string(LeafBankSpace)).NewStore(store),
		newSlot: func() *leafBankSlot {
			slot := &leafBankSlot{maxCommitNum: math.MaxUint32}
			slot.cache, _ = lru.New(perSlotLeavesCap)
			return slot
		},
	}
	b.slots, _ = lru.NewARC(slotCap)
	return b
}

// Lookup lookups a leaf from the trie named name by the given leafKey.
func (b *LeafBank) Lookup(name string, leafKey []byte) (*Leaf, error) {
	// get slot from slots cache or create a new one.
	var slot *leafBankSlot
	if cached, ok := b.slots.Get(name); ok {
		slot = cached.(*leafBankSlot)
	} else {
		slot = b.newSlot()
		b.slots.Add(name, slot)
	}

	// lookup from the slot's cache if any.
	strLeafKey := string(leafKey)
	if cached, ok := slot.cache.Get(strLeafKey); ok {
		return cached.(*Leaf), nil
	}

	var leaf *Leaf
	return leaf, b.store.Snapshot(func(getter kv.Getter) error {
		getter = kv.Bucket(name).NewGetter(getter)
		maxCN := atomic.LoadUint32(&slot.maxCommitNum)
		// maxCN == MaxUint32 indicates that need to reload maxCN from store.
		// It's important to load maxCN before load leaf!
		if maxCN == math.MaxUint32 {
			if data, err := getter.Get(nil); err != nil {
				if !getter.IsNotFound(err) {
					return err
				}
				// not found
				maxCN = math.MaxUint32
			} else {
				maxCN = binary.BigEndian.Uint32(data)
			}
			atomic.StoreUint32(&slot.maxCommitNum, maxCN)
		}

		if data, err := getter.Get(leafKey); err != nil {
			if !getter.IsNotFound(err) {
				return err
			}
			// not found
			// return empty leaf. it's safe to set its commit number to max cn.
			// see VIP-212 for detail.
			leaf = &Leaf{CommitNum: maxCN}
			return nil
		} else {
			if err := rlp.DecodeBytes(data, &leaf); err != nil {
				return err
			}
			slot.cache.Add(strLeafKey, leaf)
			return nil
		}
	})
}

// Update updates a batch of leaves for the trie named name.
func (b *LeafBank) Update(name string, commitNum uint32, batch func(save SaveLeaf) error) (err error) {
	var slot *leafBankSlot
	if cached, ok := b.slots.Get(name); ok {
		slot = cached.(*leafBankSlot)
		defer func() {
			if err == nil {
				// make maxCommitNum to be reloaded in next access.
				atomic.StoreUint32(&slot.maxCommitNum, math.MaxUint32)
			}
		}()
	}

	return b.store.Batch(func(putter kv.Putter) error {
		putter = kv.Bucket(name).NewPutter(putter)

		if err := batch(func(key, value, meta []byte) error {
			data, err := rlp.EncodeToBytes(&Leaf{
				value, meta,
				commitNum,
			})
			if err != nil {
				return err
			}
			if slot != nil {
				// invalidate cached leaves
				slot.cache.Remove(string(key))
			}
			return putter.Put(key, data)
		}); err != nil {
			return err
		}
		// at last, save the commit number as max commit number.
		return putter.Put(nil, appendUint32(nil, commitNum))
	})
}

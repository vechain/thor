// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/trie"
)

// storageLeaf is the entity stored in leaf bank.
type storageLeaf struct {
	*trie.Leaf
	CommitNum uint32
}

// SaveLeaf defines the function to save the leaf.
type SaveLeaf func(key []byte, leaf *trie.Leaf) error

// leafBankSlot presents per-trie state and cached leaves.
type leafBankSlot struct {
	maxCommitNum uint32     // max recorded commit number
	cache        *lru.Cache // cached leaves
}

// LeafBank records accumulated trie leaves to help accelerate trie leaf access
// according to VIP-212.
type LeafBank struct {
	store kv.Store
	slots *lru.ARCCache
}

// NewLeafBank creates a new LeafBank instance.
// The slotCap indicates the capacity of cached per-trie slots.
func NewLeafBank(store kv.Store, slotCap int) *LeafBank {
	b := &LeafBank{
		store: kv.Bucket(string(LeafBankSpace)).NewStore(store),
	}
	b.slots, _ = lru.NewARC(slotCap)
	return b
}

func (b *LeafBank) newSlot(name string) (*leafBankSlot, error) {
	getter := kv.Bucket(name).NewGetter(b.store)
	if data, err := getter.Get(nil); err != nil {
		if !getter.IsNotFound(err) {
			return nil, err
		}
		// the trie has no leaf recorded yet
		return nil, nil
	} else {
		slot := &leafBankSlot{maxCommitNum: binary.BigEndian.Uint32(data)}
		slot.cache, _ = lru.New(32)
		return slot, nil
	}
}

// Lookup lookups a leaf from the trie named name by the given leafKey.
// The returned leaf might be nil if no leaf recorded yet.
// The commitNum indicates up to which commit number the leaf is valid.
func (b *LeafBank) Lookup(name string, leafKey []byte) (leaf *trie.Leaf, commitNum uint32, err error) {
	// get slot from slots cache or create a new one.
	var slot *leafBankSlot
	if cached, ok := b.slots.Get(name); ok {
		slot = cached.(*leafBankSlot)
	} else {
		if slot, err = b.newSlot(name); err != nil {
			return nil, 0, err
		}
		if slot == nil {
			// the trie has no leaf recorded yet
			return nil, 0, nil
		}
		b.slots.Add(name, slot)
	}

	// lookup from the slot's cache if any.
	strLeafKey := string(leafKey)
	if cached, ok := slot.cache.Get(strLeafKey); ok {
		sLeaf := cached.(*storageLeaf)
		return sLeaf.Leaf, sLeaf.CommitNum, nil
	}

	getter := kv.Bucket(name).NewGetter(b.store)
	if data, err := getter.Get(leafKey); err != nil {
		if !getter.IsNotFound(err) {
			return nil, 0, err
		}
		// not found
		// return empty leaf with max commit number.
		return &trie.Leaf{}, atomic.LoadUint32(&slot.maxCommitNum), nil
	} else {
		var sLeaf storageLeaf
		if err := rlp.DecodeBytes(data, &sLeaf); err != nil {
			return nil, 0, err
		}
		slot.cache.Add(strLeafKey, &sLeaf)
		return sLeaf.Leaf, sLeaf.CommitNum, nil
	}
}

// Update saves a batch of leaves for the trie named name.
func (b *LeafBank) Update(name string, maxCommitNum uint32, batch func(save SaveLeaf) error) (err error) {
	var slot *leafBankSlot
	if cached, ok := b.slots.Get(name); ok {
		slot = cached.(*leafBankSlot)
		defer func() {
			if err == nil {
				// the slot may be evicted at this point, but it's OK that
				// newly created slot loads out-of-date maxCommitNum.
				atomic.StoreUint32(&slot.maxCommitNum, maxCommitNum)
			}
		}()
	}

	return b.store.Batch(func(putter kv.Putter) error {
		putter = kv.Bucket(name).NewPutter(putter)
		if err := batch(func(key []byte, leaf *trie.Leaf) error {
			data, err := rlp.EncodeToBytes(&storageLeaf{
				leaf,
				maxCommitNum,
			})
			if err != nil {
				return err
			}
			if slot != nil {
				// invalidate cached leaves.
				// the slot may be evicted at this point, but it's OK that
				// newly created slot loads out-of-date storageLeaf.
				slot.cache.Remove(string(key))
			}
			return putter.Put(key, data)
		}); err != nil {
			return err
		}
		// at last, save the max commit number.
		return putter.Put(nil, appendUint32(nil, maxCommitNum))
	})
}

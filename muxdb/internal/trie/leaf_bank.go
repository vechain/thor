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

const slotCacheSize = 64

// storageLeaf is the entity stored in leaf bank.
type storageLeaf struct {
	*trie.Leaf
	CommitNum uint32
}

// leafBankSlot presents per-trie state and cached leaves.
type leafBankSlot struct {
	maxCommitNum uint32     // max recorded commit number
	cache        *lru.Cache // cached leaves
	getter       kv.Getter
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
	b := &LeafBank{store: store}
	b.slots, _ = lru.NewARC(slotCap)
	return b
}

func (b *LeafBank) newSlot(name string) (*leafBankSlot, error) {
	if data, err := b.store.Get([]byte(name)); err != nil {
		if !b.store.IsNotFound(err) {
			return nil, err
		}
		// the trie has no leaf recorded yet
		return nil, nil
	} else {
		slot := &leafBankSlot{
			maxCommitNum: binary.BigEndian.Uint32(data),
			getter:       kv.Bucket(name).NewGetter(b.store)}
		slot.cache, _ = lru.New(slotCacheSize)
		return slot, nil
	}
}

// Lookup lookups a leaf from the trie named name by the given leafKey.
// The returned leaf might be nil if no leaf recorded yet, or deleted.
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

	buf := bufferPool.Get().(*buffer)
	defer bufferPool.Put(buf)

	if data, err := slot.getter.GetTo(leafKey, buf.buf[:0]); err != nil {
		if !slot.getter.IsNotFound(err) {
			return nil, 0, err
		}
		// not found
		// return empty leaf with max commit number.
		return &trie.Leaf{}, atomic.LoadUint32(&slot.maxCommitNum), nil
	} else {
		buf.buf = data
		// deleted
		if len(data) == 0 {
			return nil, 0, nil
		}
		var sLeaf storageLeaf
		if err := rlp.DecodeBytes(data, &sLeaf); err != nil {
			return nil, 0, err
		}
		slot.cache.Add(strLeafKey, &sLeaf)
		return sLeaf.Leaf, sLeaf.CommitNum, nil
	}
}

// NewUpdater creates the leaf updater for a trie with the given name.
func (b *LeafBank) NewUpdater(name string, rootCommitNum uint32) *LeafUpdater {
	var slot *leafBankSlot
	if cached, ok := b.slots.Get(name); ok {
		slot = cached.(*leafBankSlot)
	}
	bulk := kv.Bucket(name).NewStore(b.store).Bulk()
	bulk.EnableAutoFlush()
	return &LeafUpdater{
		slot:          slot,
		bulk:          bulk,
		rootCommitNum: rootCommitNum,
	}
}

// LeafUpdater helps to record trie leaves.
type LeafUpdater struct {
	slot          *leafBankSlot // might be nil
	bulk          kv.Bulk
	rootCommitNum uint32
}

// Update updates the leaf for the given key.
func (u *LeafUpdater) Update(key []byte, leaf *trie.Leaf) error {
	data, err := rlp.EncodeToBytes(&storageLeaf{
		leaf,
		u.rootCommitNum, // inherits root's commit number
	})
	if err != nil {
		return err
	}
	if u.slot != nil {
		// invalidate cached leaves.
		// the slot may be evicted at this point, but it's OK that
		// newly created slot loads out-of-date storageLeaf.
		u.slot.cache.Remove(string(key))
	}
	return u.bulk.Put(key, data)
}

// Commit commits updates into leafbank.
func (u *LeafUpdater) Commit() error {
	if err := u.bulk.Put(nil, appendUint32(nil, u.rootCommitNum)); err != nil {
		return err
	}
	if err := u.bulk.Write(); err != nil {
		return err
	}
	if u.slot != nil {
		// the slot may be evicted at this point, but it's OK that
		// newly created slot loads out-of-date maxCommitNum.
		atomic.StoreUint32(&u.slot.maxCommitNum, u.rootCommitNum)
	}
	return nil
}

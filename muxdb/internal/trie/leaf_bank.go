// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/trie"
)

const slotCacheSize = 64

// LeafRecord is the entity stored in leaf bank.
type LeafRecord struct {
	Value, Meta []byte
	CommitNum   uint32
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
	space byte
	slots *lru.ARCCache
}

// NewLeafBank creates a new LeafBank instance.
// The slotCap indicates the capacity of cached per-trie slots.
func NewLeafBank(store kv.Store, space byte, slotCap int) *LeafBank {
	b := &LeafBank{store: store, space: space}
	b.slots, _ = lru.NewARC(slotCap)
	return b
}

func (b *LeafBank) newSlot(name string) (*leafBankSlot, error) {
	getter := kv.Bucket(string(b.space) + name).NewGetter(b.store)
	var maxCommitNum uint32
	if data, err := getter.Get(nil); err != nil {
		if !getter.IsNotFound(err) {
			return nil, errors.Wrap(err, "get from leafbank")
		}
	} else {
		maxCommitNum = binary.BigEndian.Uint32(data)
	}

	slot := &leafBankSlot{
		maxCommitNum: maxCommitNum,
		getter:       getter}
	slot.cache, _ = lru.New(slotCacheSize)
	return slot, nil
}

// Lookup lookups a leaf from the trie named name by the given leafKey.
// The returned leaf might be nil (empty value) if the key is never seen (with max slot commitNum),
// or was ever seen but can't be located now (touched, with commitNum == 0).
// LeafRecord.CommitNum indicates up to which commit number the leaf is valid.
func (b *LeafBank) Lookup(name string, leafKey []byte) (rec *LeafRecord, err error) {
	// get slot from slots cache or create a new one.
	var slot *leafBankSlot
	if cached, ok := b.slots.Get(name); ok {
		slot = cached.(*leafBankSlot)
	} else {
		var err error
		if slot, err = b.newSlot(name); err != nil {
			return nil, err
		}
		b.slots.Add(name, slot)
	}

	// lookup from the slot's cache if any.
	strLeafKey := string(leafKey)
	if cached, ok := slot.cache.Get(strLeafKey); ok {
		rec := cached.(*LeafRecord)
		return rec, nil
	}

	defer func() {
		if err == nil {
			slot.cache.Add(strLeafKey, rec)
		}
	}()

	buf := bufferPool.Get().(*buffer)
	defer bufferPool.Put(buf)

	if data, err := slot.getter.GetTo(leafKey, buf.buf[:0]); err != nil {
		if !slot.getter.IsNotFound(err) {
			return nil, errors.Wrap(err, "get from leafbank")
		}
		// not seen till slot.maxCommitNum
		return &LeafRecord{CommitNum: atomic.LoadUint32(&slot.maxCommitNum)}, nil
	} else {
		buf.buf = data
		if len(data) > 0 {
			var rec LeafRecord
			if err := rlp.DecodeBytes(data, &rec); err != nil {
				panic(errors.Wrap(err, "decode leaf record"))
			}
			if len(rec.Meta) == 0 {
				rec.Meta = nil // normalize
			}
			return &rec, nil
		} else {
			// ever seen, but can't be located now (touched)
			return &LeafRecord{}, nil
		}
	}
}

// Touch touches the key. Now it just sets an empty value for the given key.
// Later if the engine supports conditional update, we can do real touch.
//
// In practical, the leaf bank is not updated every commit. Suppose a key is set and
// is soon deleted, it might be missing in leaf bank records. In this case,
// inexistence assertion of the leaf bank is incorrect. To keep the integrity of the leaf bank,
// this method should be called everytime a key is deleted from the trie.
func (b *LeafBank) Touch(putter kv.Putter, name string, key []byte) error {
	buf := bufferPool.Get().(*buffer)
	defer bufferPool.Put(buf)

	buf.buf = append(buf.buf[:0], b.space)
	buf.buf = append(buf.buf, name...)
	buf.buf = append(buf.buf, key...)

	return putter.Put(buf.buf, nil)
}

// NewUpdater creates the leaf updater for a trie with the given name.
func (b *LeafBank) NewUpdater(name string, rootCommitNum uint32) *LeafUpdater {
	var slot *leafBankSlot
	if cached, ok := b.slots.Get(name); ok {
		slot = cached.(*leafBankSlot)
	}
	bulk := kv.Bucket(string(b.space) + name).NewStore(b.store).Bulk()
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
	rec := LeafRecord{
		Value:     leaf.Value,
		Meta:      leaf.Meta,
		CommitNum: u.rootCommitNum, // inherits root's commit number
	}
	data, err := rlp.EncodeToBytes(&rec)
	if err != nil {
		return err
	}
	if u.slot != nil {
		strKey := string(key)
		if u.slot.cache.Contains(strKey) {
			// update cached records.
			u.slot.cache.Add(string(key), &rec)
		}
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

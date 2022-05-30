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

const (
	entityPrefix          = "e"
	deletionJournalPrefix = "d"

	slotCacheSize = 64
)

// LeafRecord presents the queried leaf record.
type LeafRecord struct {
	*trie.Leaf
	CommitNum     uint32 // which commit number the leaf was committed
	SlotCommitNum uint32 // up to which commit number this leaf is valid
}

// leafEntity is the entity stored in leaf bank.
type leafEntity struct {
	*trie.Leaf `rlp:"nil"`
	CommitNum  uint32
}

var encodedEmptyLeafEntity, _ = rlp.EncodeToBytes(&leafEntity{})

// trieSlot holds the state of a trie slot.
type trieSlot struct {
	getter    kv.Getter
	commitNum uint32 // the commit number of this slot
	cache     *lru.Cache
}

func (s *trieSlot) getEntity(key []byte) (*leafEntity, error) {
	data, err := s.getter.Get(key)
	if err != nil {
		if !s.getter.IsNotFound(err) {
			return nil, errors.Wrap(err, "get entity from leafbank")
		}
		// never seen, which means it has been an empty leaf until slotCommitNum.
		return nil, nil
	}

	// entity found
	var ent leafEntity
	if err := rlp.DecodeBytes(data, &ent); err != nil {
		return nil, errors.Wrap(err, "decode leaf entity")
	}

	if ent.Leaf != nil && len(ent.Leaf.Meta) == 0 {
		ent.Meta = nil // normalize
	}
	return &ent, nil
}

func (s *trieSlot) getRecord(key []byte) (rec *LeafRecord, err error) {
	slotCommitNum := atomic.LoadUint32(&s.commitNum)
	if slotCommitNum == 0 {
		// an empty slot always gives undetermined value.
		return &LeafRecord{}, nil
	}

	strKey := string(key)
	if cached, ok := s.cache.Get(strKey); ok {
		return cached.(*LeafRecord), nil
	}

	defer func() {
		if err == nil {
			s.cache.Add(strKey, rec)
		}
	}()

	ent, err := s.getEntity(key)
	if err != nil {
		return nil, err
	}

	if ent == nil { // never seen
		return &LeafRecord{
			Leaf:          &trie.Leaf{},
			CommitNum:     0,
			SlotCommitNum: slotCommitNum,
		}, nil
	}

	if slotCommitNum < ent.CommitNum {
		slotCommitNum = ent.CommitNum
	}

	return &LeafRecord{
		Leaf:          ent.Leaf,
		CommitNum:     ent.CommitNum,
		SlotCommitNum: slotCommitNum,
	}, nil
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

func (b *LeafBank) slotBucket(name string) kv.Bucket {
	return kv.Bucket(string(b.space) + entityPrefix + name)
}

func (b *LeafBank) deletionJournalBucket(name string) kv.Bucket {
	return kv.Bucket(string(b.space) + deletionJournalPrefix + name)
}

// getSlot gets slot from slots cache or create a new one.
func (b *LeafBank) getSlot(name string) (*trieSlot, error) {
	if cached, ok := b.slots.Get(name); ok {
		return cached.(*trieSlot), nil
	}

	slot := &trieSlot{getter: b.slotBucket(name).NewGetter(b.store)}
	if data, err := slot.getter.Get(nil); err != nil {
		if !slot.getter.IsNotFound(err) {
			return nil, errors.Wrap(err, "get slot from leafbank")
		}
	} else {
		slot.commitNum = binary.BigEndian.Uint32(data)
	}

	slot.cache, _ = lru.New(slotCacheSize)
	b.slots.Add(name, slot)
	return slot, nil
}

// Lookup lookups a leaf record by the given leafKey for the trie named by name.
// LeafRecord.Leaf might be nil if the leaf can't be determined.
func (b *LeafBank) Lookup(name string, leafKey []byte) (rec *LeafRecord, err error) {
	slot, err := b.getSlot(name)
	if err != nil {
		return nil, err
	}
	return slot.getRecord(leafKey)
}

// LogDeletions saves the journal of leaf-key deletions which issued by one trie-commit.
func (b *LeafBank) LogDeletions(putter kv.Putter, name string, keys []string, commitNum uint32) error {
	if len(keys) == 0 {
		return nil
	}

	bkt := b.deletionJournalBucket(name) + kv.Bucket(appendUint32(nil, commitNum))
	putter = bkt.NewPutter(putter)
	for _, k := range keys {
		if err := putter.Put([]byte(k), nil); err != nil {
			return err
		}
	}
	return nil
}

// NewUpdater creates a leaf-updater for a trie slot with the given name.
func (b *LeafBank) NewUpdater(name string, baseCommitNum, targetCommitNum uint32) (*LeafUpdater, error) {
	slot, err := b.getSlot(name)
	if err != nil {
		return nil, err
	}

	bulk := b.slotBucket(name).
		NewStore(b.store).
		Bulk()
	bulk.EnableAutoFlush()

	// traverse the deletion-journal and write to the slot
	iter := b.deletionJournalBucket(name).
		NewStore(b.store).
		Iterate(kv.Range{
			Start: appendUint32(nil, baseCommitNum),
			Limit: appendUint32(nil, targetCommitNum+1),
		})
	defer iter.Release()
	for iter.Next() {
		// skip commit number to get leaf key
		leafKey := iter.Key()[4:]
		// put empty value to mark the leaf to undetermined state
		if err := bulk.Put(leafKey, encodedEmptyLeafEntity); err != nil {
			return nil, err
		}
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}

	return &LeafUpdater{
		slot:            slot,
		bulk:            bulk,
		targetCommitNum: targetCommitNum,
	}, nil
}

// LeafUpdater helps to record trie leaves.
type LeafUpdater struct {
	slot            *trieSlot
	bulk            kv.Bulk
	targetCommitNum uint32
}

// Update updates the leaf for the given key.
func (u *LeafUpdater) Update(leafKey []byte, leaf *trie.Leaf, leafCommitNum uint32) error {
	ent := &leafEntity{
		Leaf:      leaf,
		CommitNum: leafCommitNum,
	}
	data, err := rlp.EncodeToBytes(ent)
	if err != nil {
		return err
	}

	return u.bulk.Put(leafKey, data)
}

// Commit commits updates into leafbank.
func (u *LeafUpdater) Commit() error {
	// save slot commit number
	if err := u.bulk.Put(nil, appendUint32(nil, u.targetCommitNum)); err != nil {
		return err
	}
	if err := u.bulk.Write(); err != nil {
		return err
	}
	atomic.StoreUint32(&u.slot.commitNum, u.targetCommitNum)
	return nil
}

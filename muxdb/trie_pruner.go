// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package muxdb

import (
	"context"

	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

const (
	prunerBatchSize = 4096
)

// HandleTrieLeafFunc callback function to handle trie leaf.
type HandleTrieLeafFunc func(key, blob1, blob2 []byte) error

// TriePruner is the trie pruner.
type TriePruner struct {
	db *MuxDB
}

func newTriePruner(db *MuxDB) *TriePruner {
	return &TriePruner{
		db,
	}
}

// ArchiveNodes save differential nodes of two tries into permanent space.
// handleLeaf can be nil if not interested.
func (p *TriePruner) ArchiveNodes(
	ctx context.Context,
	name string,
	root1, root2 thor.Bytes32,
	handleLeaf HandleTrieLeafFunc,
) (nodeCount int, entryCount int, err error) {
	var (
		trie1 = p.db.NewTrie(name, root1)
		trie2 = p.db.NewTrie(name, root2)
		it, _ = trie.NewDifferenceIterator(trie1.NodeIterator(nil), trie2.NodeIterator(nil))
	)

	err = p.db.engine.Batch(func(putter kv.PutFlusher) error {
		keyBuf := newTrieNodeKeyBuf(name)
		for it.Next(true) {
			if h := it.Hash(); !h.IsZero() {
				enc, err := it.Node()
				if err != nil {
					return err
				}
				nodeKey := &trie.NodeKey{
					Hash: h[:],
					Path: it.Path(),
				}
				if err := keyBuf.Put(putter.Put, nodeKey, enc, trieSpaceP); err != nil {
					return err
				}
				nodeCount++
				if nodeCount > 0 && nodeCount%prunerBatchSize == 0 {
					if err := putter.Flush(); err != nil {
						return err
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}
				}
			}

			if it.Leaf() {
				entryCount++
				if handleLeaf != nil {
					blob1, err := trie1.Get(it.LeafKey())
					if err != nil {
						return err
					}
					blob2 := it.LeafBlob()
					if err := handleLeaf(it.LeafKey(), blob1, blob2); err != nil {
						return err
					}
				}
			}
		}
		return it.Error()
	})
	return
}

// DropStaleNodes delete stale trie nodes.
func (p *TriePruner) DropStaleNodes(ctx context.Context) (count int, err error) {
	err = p.db.engine.Batch(func(putter kv.PutFlusher) error {
		rng := kv.Range(*util.BytesPrefix([]byte{p.db.trieLiveSpace.Stale()}))
		var nextStart []byte
		for {
			iterCount := 0
			// use short-range iterator here to prevent from holding snapshot for long time.
			if err := p.db.engine.Iterate(rng, func(pair kv.Pair) bool {
				iterCount++
				nextStart = append(append(nextStart[:0], pair.Key()...), 0)

				// error can be ignored here
				_ = putter.Delete(pair.Key())

				return iterCount < prunerBatchSize
			}); err != nil {
				return err
			}

			// no more
			if iterCount == 0 {
				break
			}
			count += iterCount
			if err := putter.Flush(); err != nil {
				return err
			}
			rng.Start = nextStart

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		return nil
	})
	return
}

// SwitchLiveSpace switch trie live space.
func (p *TriePruner) SwitchLiveSpace() error {
	return p.db.trieLiveSpace.Switch()
}

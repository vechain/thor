// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"encoding/binary"

	"github.com/vechain/thor/v2/kv"
	"github.com/vechain/thor/v2/thor"
)

// currentResyncVersion is bumped per new resync pass so startup skips done work.
const currentResyncVersion = uint32(1)

var resyncVersionKey = []byte("bft.resync.version")

func saveResyncVersion(putter kv.Putter, version uint32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], version)
	return putter.Put(resyncVersionKey, b[:])
}

func loadResyncVersion(store kv.Store) (uint32, error) {
	b, err := store.Get(resyncVersionKey)
	if err != nil {
		if store.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

func saveQuality(putter kv.Putter, id thor.Bytes32, quality uint32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], quality)

	return putter.Put(id.Bytes(), b[:])
}

func loadQuality(getter kv.Getter, id thor.Bytes32) (uint32, error) {
	b, err := getter.Get(id.Bytes())
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint32(b), nil
}

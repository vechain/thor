// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"encoding/binary"
	"encoding/json"

	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/thor"
)

func saveWeight(putter kv.Putter, id thor.Bytes32, weight uint32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], weight)

	return putter.Put(id.Bytes(), b[:])
}

func loadWeight(getter kv.Getter, id thor.Bytes32) (uint32, error) {
	b, err := getter.Get(id.Bytes())
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint32(b), nil
}

func saveVoted(putter kv.Putter, voted map[thor.Bytes32]uint32) error {
	b, err := json.Marshal(voted)
	if err != nil {
		return nil
	}

	return putter.Put(votedKey, b)
}

func loadVoted(getter kv.Getter) (map[thor.Bytes32]uint32, error) {
	voted := make(map[thor.Bytes32]uint32)

	b, err := getter.Get(votedKey)
	if err != nil {
		if getter.IsNotFound(err) {
			return voted, nil
		}
		return nil, err
	}

	err = json.Unmarshal(b, &voted)
	if err != nil {
		return nil, err
	}

	return voted, nil
}

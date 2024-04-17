// Copyright (c) 2023 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"testing"

	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/trie"
)

func TestStater(t *testing.T) {
	db := muxdb.NewMem()
	stater := NewStater(db)

	// Example State
	var root trie.Root
	root.Ver.Major = 1

	state := stater.NewState(root)

	if state == nil {
		t.Errorf("NewState returned nil")
	}
}

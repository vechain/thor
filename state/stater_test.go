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

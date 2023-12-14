package state

import (
	"testing"

	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
)

func TestStater(t *testing.T) {
	db := muxdb.NewMem()
	stater := NewStater(db)

	// Example State
	root := thor.Bytes32{}
	blockNum := uint32(1)
	blockConflicts := uint32(0)
	steadyBlockNum := uint32(1)

	state := stater.NewState(root, blockNum, blockConflicts, steadyBlockNum)

	if state == nil {
		t.Errorf("NewState returned nil")
	}
}

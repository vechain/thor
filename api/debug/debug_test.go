package debug

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

func TestHandleRevision(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := chain.NewRepository(db, b)

	dbg := New(repo, stater, thor.ForkConfig{}, 100000000, true)
	chainSummary, err := dbg.handleRevision("latest")

	assert.NotNil(t, chainSummary)
	assert.Nil(t, err)
}

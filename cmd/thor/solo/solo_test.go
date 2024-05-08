package solo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

func newSolo() *Solo {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()
	logDb, _ := logdb.NewMem()
	b, _, _, _ := gene.Build(stater)
	repo, _ := chain.NewRepository(db, b)
	mempool := txpool.New(repo, stater, txpool.Options{Limit: 10000, LimitPerAccount: 16, MaxLifetime: 10 * time.Minute})

	return New(repo, stater, logDb, mempool, 0, false, false, thor.ForkConfig{})
}

func TestInitSolo(t *testing.T) {
	solo := newSolo()

	err := solo.initSolo()
	assert.Nil(t, err)

	bestBlock := solo.repo.BestBlockSummary()
	assert.Equal(t, len(bestBlock.Txs), 1)
}

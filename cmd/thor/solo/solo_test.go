// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solo

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin"
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
	mempool := txpool.New(repo, stater, txpool.Options{Limit: 10000, LimitPerAccount: 16, MaxLifetime: 10 * time.Minute}, &thor.NoFork)

	opts := Options{
		GasLimit:         0,
		SkipLogs:         false,
		MinTxPriorityFee: 0,
		OnDemand:         true,
		BlockInterval:    thor.BlockInterval,
	}

	engine := NewCore(repo, stater, logDb, opts, &thor.ForkConfig{GALACTICA: math.MaxUint32, HAYABUSA_TP: 1})

	return New(repo, stater, mempool, opts, engine)
}

func TestInitSolo(t *testing.T) {
	solo := newSolo()

	// init solo -> this should mine a block with the gas price tx
	err := solo.init(context.Background())
	assert.Nil(t, err)

	// check the gas price
	best := solo.repo.BestBlockSummary()
	newState := solo.stater.NewState(best.Root())
	currentBGP, err := builtin.Params.Native(newState).Get(thor.KeyLegacyTxBaseGasPrice)
	assert.Nil(t, err)
	assert.Equal(t, baseGasPrice, currentBGP)
}

// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"fmt"
	"testing"
	"time"

	comm2 "github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/logdb"

	bft2 "github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/txpool"

	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const (
	LIMIT             = 10
	LIMIT_PER_ACCOUNT = 10
)

func getFlowAndNode(t *testing.T, forkConfig *thor.ForkConfig) (*packer.Flow, *Node) {
	db := muxdb.NewMem()
	now := time.Now().Unix()
	launchTime := uint64(now) - thor.BlockInterval
	builder := genesis.NewDevnetWithConfig(genesis.DevConfig{ForkConfig: &thor.SoloFork, LaunchTime: launchTime})
	a1 := genesis.DevAccounts()[0]

	b0, _, _, err := builder.Build(state.NewStater(db))
	assert.Nil(t, err)

	if forkConfig == nil {
		forkConfig = &thor.SoloFork
	}

	repo, _ := chain.NewRepository(db, b0)

	pool := txpool.New(repo, state.NewStater(db), txpool.Options{
		Limit:           LIMIT,
		LimitPerAccount: LIMIT_PER_ACCOUNT,
		MaxLifetime:     time.Hour,
	}, forkConfig)
	bft, err := bft2.NewEngine(repo, db, forkConfig, a1.Address)
	assert.NoError(t, err)

	logdb, err := logdb.NewMem()
	assert.NoError(t, err)

	comm := comm2.New(repo, pool)

	n := &Node{
		txPool:     pool,
		repo:       repo,
		forkConfig: forkConfig,
		bft:        bft,
		master: &Master{
			PrivateKey:  a1.PrivateKey,
			Beneficiary: &a1.Address,
		},
		logDB:     logdb,
		logWorker: newWorker(),
		comm:      comm,
	}

	stater := state.NewStater(db)
	p := packer.New(repo, stater, a1.Address, &a1.Address, forkConfig, 0)

	minTxGas := thor.TxGas + thor.ClauseGas
	smallGasLimit := minTxGas + 1000
	flow, err := p.Mock(repo.BestBlockSummary(),
		repo.BestBlockSummary().Header.Timestamp()+thor.BlockInterval,
		smallGasLimit)
	assert.NoError(t, err)
	return flow, n
}

func TestPack(t *testing.T) {
	flow, n := getFlowAndNode(t, nil)

	bbSum := n.repo.BestBlockSummary()
	transaction1 := new(tx.Builder).
		Clause(tx.NewClause(&genesis.DevAccounts()[1].Address)).
		ChainTag(n.repo.ChainTag()).
		Expiration(32).
		BlockRef(tx.NewBlockRef(bbSum.Header.Number())).
		Gas(21000).
		Nonce(uint64(1)).
		Build()
	transaction1 = tx.MustSign(transaction1, genesis.DevAccounts()[0].PrivateKey)
	err := n.txPool.Add(transaction1)
	assert.NoError(t, err)

	transaction2 := new(tx.Builder).
		Clause(tx.NewClause(&genesis.DevAccounts()[1].Address)).
		ChainTag(n.repo.ChainTag()).
		Expiration(32).
		BlockRef(tx.NewBlockRef(bbSum.Header.Number())).
		Gas(21000).
		Build()
	transaction2 = tx.MustSign(transaction2, genesis.DevAccounts()[0].PrivateKey)
	err = n.txPool.Add(transaction2)
	assert.NoError(t, err)

	time.Sleep(1100 * time.Millisecond)

	err = n.pack(flow)
	assert.NoError(t, err)
}

func TestCleanupTransactions(t *testing.T) {
	n := &Node{}
	var txsToRemove []*tx.Transaction

	assert.NotPanics(t, func() {
		n.cleanupTransactions(txsToRemove)
	})
}

func TestUpdatePackMetrics(t *testing.T) {
	tests := []struct {
		name    string
		success bool
	}{
		{
			name:    "successful pack",
			success: true,
		},
		{
			name:    "failed pack",
			success: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{}

			for range 3 {
				assert.NotPanics(t, func() {
					n.updatePackMetrics(tt.success)
				})
			}
		})
	}
}

func TestPackContext(t *testing.T) {
	flow := &packer.Flow{}
	oldBest := &chain.BlockSummary{}

	ctx := &packContext{
		flow:       flow,
		conflicts:  0,
		startTime:  mclock.Now(),
		logEnabled: true,
		oldBest:    oldBest,
	}

	assert.NotNil(t, ctx)
	assert.Equal(t, flow, ctx.flow)
	assert.Equal(t, uint32(0), ctx.conflicts)
	assert.True(t, ctx.logEnabled)
	assert.Equal(t, oldBest, ctx.oldBest)
}

func TestWriteLogsIfEnabled(t *testing.T) {
	tests := []struct {
		name       string
		logEnabled bool
	}{
		{
			name:       "logs disabled - should return nil",
			logEnabled: false,
		},
	}

	parentID := thor.Bytes32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{}

			ctx := &packContext{
				logEnabled: tt.logEnabled,
				oldBest: &chain.BlockSummary{
					Header: (&block.Builder{}).
						ParentID(parentID).
						Build().Header(),
					Txs:       make([]thor.Bytes32, 0),
					Size:      0,
					Conflicts: 0,
				},
			}

			newBlock := (&block.Builder{}).
				ParentID(thor.Bytes32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}).
				Timestamp(uint64(time.Now().Unix())).
				Build()
			receipts := tx.Receipts{}

			err := n.writeLogsIfEnabled(ctx, newBlock, receipts)
			assert.NoError(t, err)
		})
	}
}

func TestSyncLogWorker(t *testing.T) {
	tests := []struct {
		name       string
		logEnabled bool
	}{
		{
			name:       "logs disabled",
			logEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{}

			ctx := &packContext{
				logEnabled: tt.logEnabled,
			}

			err := n.syncLogWorker(ctx)
			assert.NoError(t, err)
		})
	}
}

func TestDetermineVotingRequirement_BeforeFinality(t *testing.T) {
	flow, n := getFlowAndNode(t, nil)

	n.forkConfig = &thor.ForkConfig{FINALITY: 1000}

	shouldVote, err := n.determineVotingRequirement(flow)

	assert.NoError(t, err)
	assert.False(t, shouldVote)
}

func TestProcessPackedBlock_WriteLogsError(t *testing.T) {
	flow, n := getFlowAndNode(t, nil)

	ctx := &packContext{
		flow:       flow,
		conflicts:  0,
		startTime:  mclock.Now(),
		logEnabled: true,
		oldBest:    n.repo.BestBlockSummary(),
	}

	newBlock := (&block.Builder{}).
		ParentID(thor.Bytes32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}).
		Timestamp(uint64(time.Now().Unix())).
		Build()
	receipts := tx.Receipts{}

	n.logDBFailed = true

	stage := &state.Stage{}

	err := n.processPackedBlock(ctx, newBlock, stage, receipts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "write logs")
}

func TestCommitToBFT_BeforeFinality(t *testing.T) {
	forkConfig := thor.SoloFork
	forkConfig.FINALITY = 1000
	_, n := getFlowAndNode(t, &forkConfig)

	newBlock := (&block.Builder{}).
		ParentID(n.repo.BestBlockSummary().Header.ID()).
		Timestamp(uint64(time.Now().Unix())).
		Build()

	err := n.commitToBFT(newBlock)

	assert.NoError(t, err)
	assert.Nil(t, err)
}

func TestSyncLogWorker_SyncLogWorkerFail(t *testing.T) {
	flow, n := getFlowAndNode(t, nil)

	ctx := &packContext{
		flow:       flow,
		conflicts:  0,
		startTime:  mclock.Now(),
		logEnabled: true,
		oldBest:    n.repo.BestBlockSummary(),
	}

	assert.False(t, n.logDBFailed)

	n.logWorker.Run(func() error {
		return fmt.Errorf("error")
	})

	err := n.syncLogWorker(ctx)
	assert.NoError(t, err)

	assert.True(t, n.logDBFailed)
}

func TestCleanupTransactions_WithTransactions(t *testing.T) {
	forkConfig := thor.SoloFork
	forkConfig.GALACTICA = 1000
	_, n := getFlowAndNode(t, &forkConfig)

	bbSum := n.repo.BestBlockSummary()

	transactions := []*tx.Transaction{}

	for idx := range 3 {
		trx := new(tx.Builder).
			Clause(tx.NewClause(&genesis.DevAccounts()[1].Address)).
			ChainTag(n.repo.ChainTag()).
			Expiration(32).
			BlockRef(tx.NewBlockRef(bbSum.Header.Number())).
			Gas(21000).
			Nonce(uint64(idx + 1)).
			Build()
		trx = tx.MustSign(trx, genesis.DevAccounts()[1].PrivateKey)
		transactions = append(transactions, trx)
	}

	for _, tx := range transactions {
		err := n.txPool.Add(tx)
		assert.NoError(t, err)
	}

	time.Sleep(1100 * time.Millisecond)
	assert.Equal(t, len(transactions), n.txPool.Len())

	txsToRemove := transactions[:2]

	n.cleanupTransactions(txsToRemove)

	assert.Equal(t, 1, n.txPool.Len())

	for _, tx := range txsToRemove {
		assert.Nil(t, n.txPool.Get(tx.ID()))
	}

	assert.NotNil(t, n.txPool.Get(transactions[2].ID()))
}

func TestPackerLoop_ContextCancellation(t *testing.T) {
	flow, n := getFlowAndNode(t, nil)
	_ = flow

	t.Run("immediate_cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		n.packerLoop(ctx)
	})

	t.Run("delayed_cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		n.packerLoop(ctx)
	})

	t.Run("longer_delayed_cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(200 * time.Millisecond)
			cancel()
		}()

		n.packerLoop(ctx)
	})
}

func TestPackerLoop_WithTimeout(t *testing.T) {
	flow, n := getFlowAndNode(t, nil)
	_ = flow

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	n.packerLoop(ctx)
}

func TestPackerLoop_WithDeadline(t *testing.T) {
	flow, n := getFlowAndNode(t, nil)
	_ = flow

	deadline := time.Now().Add(100 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	n.packerLoop(ctx)
}

// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"math/big"
	"math/rand/v2"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func TestProcessFork_ReinjectsSideChainTxs(t *testing.T) {
	repo, sideTx := buildForkRepo(t)
	pool := &mockTxPool{}
	n := &Node{repo: repo, txPool: pool}

	b0 := repo.GenesisBlock()
	oldBest := mustGetBestChild(t, repo, b0.Header().ID()) // b1 (side chain to reinject)
	// Find the competing branch tip's child candidate: new best parent is b1x.
	conflicts, err := repo.GetConflicts(1)
	require.NoError(t, err)
	require.Len(t, conflicts, 2)

	var newParentID thor.Bytes32
	for _, id := range conflicts {
		if id != oldBest.Header().ID() {
			newParentID = id
			break
		}
	}
	require.False(t, newParentID.IsZero())

	newBest := signBlock(t, new(block.Builder).
		ParentID(newParentID).
		Timestamp(oldBest.Header().Timestamp()+thor.BlockInterval()).
		TotalScore(oldBest.Header().TotalScore()+1).
		GasLimit(oldBest.Header().GasLimit()).
		Build())

	n.processFork(newBest, oldBest.Header().ID())

	calls := pool.getAdmitCalls()
	require.Len(t, calls, 1, "exactly one side-chain tx should be reinjected")
	assert.Equal(t, "ReinjectFromFork", calls[0].method)
	assert.Equal(t, sideTx.ID(), calls[0].txID)

	// A transaction present on both fork arms is not lost, but the complete new
	// arm (including the uncommitted tip argument) is still passed for nonce reset.
	sameTxTip := signBlock(t, new(block.Builder).
		ParentID(newParentID).
		Timestamp(oldBest.Header().Timestamp()+thor.BlockInterval()).
		TotalScore(oldBest.Header().TotalScore()+1).
		GasLimit(oldBest.Header().GasLimit()).
		Transaction(sideTx).
		Build())
	pool = &mockTxPool{}
	n = &Node{repo: repo, txPool: pool}
	n.processFork(sameTxTip, oldBest.Header().ID())
	assert.Empty(t, pool.getAdmitCalls())
	forks := pool.getForkCalls()
	require.Len(t, forks, 1)
	assert.Empty(t, forks[0].Discarded)
	require.Len(t, forks[0].Included, 1)
	assert.Equal(t, sideTx.ID(), forks[0].Included[0].ID())
}

func TestProcessFork_NoSideChainIsNoOp(t *testing.T) {
	db := muxdb.NewMem()
	gene := new(genesis.Builder).
		GasLimit(thor.InitialGasLimit).
		Timestamp(uint64(1_700_000_000)).
		ForkConfig(&thor.NoFork).
		State(func(st *state.State) error {
			bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
			for _, acc := range genesis.DevAccounts() {
				st.SetBalance(acc.Address, bal)
				st.SetEnergy(acc.Address, bal, uint64(1_700_000_000))
			}
			return nil
		})
	b0, _, _, err := gene.Build(state.NewStater(db))
	require.NoError(t, err)
	repo, err := chain.NewRepository(db, b0)
	require.NoError(t, err)

	b1 := signBlock(t, new(block.Builder).
		ParentID(b0.Header().ID()).
		Timestamp(b0.Header().Timestamp()+thor.BlockInterval()).
		TotalScore(100).
		GasLimit(b0.Header().GasLimit()).
		Build())
	require.NoError(t, repo.AddBlock(b1, nil, 0, true))

	b2 := signBlock(t, new(block.Builder).
		ParentID(b1.Header().ID()).
		Timestamp(b1.Header().Timestamp()+thor.BlockInterval()).
		TotalScore(200).
		GasLimit(b1.Header().GasLimit()).
		Build())

	pool := &mockTxPool{}
	n := &Node{repo: repo, txPool: pool}
	n.processFork(b2, b1.Header().ID())

	assert.Empty(t, pool.getAdmitCalls(), "canonical extension must not reinject txs")
}

func buildForkRepo(t *testing.T) (*chain.Repository, *tx.Transaction) {
	t.Helper()

	db := muxdb.NewMem()
	launch := uint64(1_700_000_000)
	gene := new(genesis.Builder).
		GasLimit(thor.InitialGasLimit).
		Timestamp(launch).
		ForkConfig(&thor.NoFork).
		State(func(st *state.State) error {
			bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
			for _, acc := range genesis.DevAccounts() {
				st.SetBalance(acc.Address, bal)
				st.SetEnergy(acc.Address, bal, launch)
			}
			return nil
		})
	b0, _, _, err := gene.Build(state.NewStater(db))
	require.NoError(t, err)
	repo, err := chain.NewRepository(db, b0)
	require.NoError(t, err)

	acc := genesis.DevAccounts()[0]
	sideTx := tx.MustSign(
		tx.NewBuilder(tx.TypeLegacy).
			ChainTag(repo.ChainTag()).
			BlockRef(tx.NewBlockRef(0)).
			Expiration(100).
			Gas(21000).
			Nonce(rand.Uint64()). //#nosec G404
			Clause(tx.NewClause(&acc.Address).WithValue(big.NewInt(1))).
			Build(),
		acc.PrivateKey,
	)

	// Old best branch containing the tx that should be reinjected.
	b1 := signBlock(t, new(block.Builder).
		ParentID(b0.Header().ID()).
		Timestamp(b0.Header().Timestamp()+thor.BlockInterval()).
		TotalScore(100).
		GasLimit(b0.Header().GasLimit()).
		Transaction(sideTx).
		Build())
	require.NoError(t, repo.AddBlock(b1, tx.Receipts{&tx.Receipt{}}, 0, true))

	// Competing sibling at the same height (side branch that will become canonical parent).
	b1x := signBlock(t, new(block.Builder).
		ParentID(b0.Header().ID()).
		Timestamp(b0.Header().Timestamp()+thor.BlockInterval()+1).
		TotalScore(100).
		GasLimit(b0.Header().GasLimit()).
		Build())
	require.NoError(t, repo.AddBlock(b1x, nil, 1, false))

	return repo, sideTx
}

func mustGetBestChild(t *testing.T, repo *chain.Repository, parentID thor.Bytes32) *block.Block {
	t.Helper()
	best := repo.BestBlockSummary()
	require.Equal(t, parentID, best.Header.ParentID())
	b, err := repo.GetBlock(best.Header.ID())
	require.NoError(t, err)
	return b
}

func signBlock(t *testing.T, b *block.Block) *block.Block {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	sig, err := crypto.Sign(b.Header().SigningHash().Bytes(), key)
	require.NoError(t, err)
	return b.WithSignature(sig)
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	r "math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/upgrade/galactica"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	Tx "github.com/vechain/thor/v2/tx"
)

const LIMIT = 10
const LIMIT_PER_ACCOUNT = 2

var devAccounts = genesis.DevAccounts()

func newPool(limit int, limitPerAccount int, forkConfig *thor.ForkConfig) *TxPool {
	db := muxdb.NewMem()
	repo := newChainRepo(db)
	return New(repo, state.NewStater(db), Options{
		Limit:           limit,
		LimitPerAccount: limitPerAccount,
		MaxLifetime:     time.Hour,
	}, forkConfig)
}

func newPoolWithParams(limit int, limitPerAccount int, BlocklistCacheFilePath string, BlocklistFetchURL string, timestamp uint64, forks *thor.ForkConfig) *TxPool {
	return newPoolWithMaxLifetime(limit, limitPerAccount, BlocklistCacheFilePath, BlocklistFetchURL, timestamp, time.Hour, forks)
}

func newPoolWithMaxLifetime(limit int, limitPerAccount int, BlocklistCacheFilePath string, BlocklistFetchURL string, timestamp uint64, maxLifetime time.Duration, forks *thor.ForkConfig) *TxPool {
	db := muxdb.NewMem()
	gene := new(genesis.Builder).
		GasLimit(thor.InitialGasLimit).
		Timestamp(timestamp).
		State(func(state *state.State) error {
			bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
			for _, acc := range devAccounts {
				state.SetBalance(acc.Address, bal)
				state.SetEnergy(acc.Address, bal, timestamp)
			}
			return nil
		})
	b0, _, _, _ := gene.Build(state.NewStater(db))
	repo, _ := chain.NewRepository(db, b0)
	return New(repo, state.NewStater(db), Options{
		Limit:                  limit,
		LimitPerAccount:        limitPerAccount,
		MaxLifetime:            maxLifetime,
		BlocklistCacheFilePath: BlocklistCacheFilePath,
		BlocklistFetchURL:      BlocklistFetchURL,
	}, forks)
}

func newHTTPServer() *httptest.Server {
	// Example data to be served by the mock server
	data := "0x25Df024637d4e56c1aE9563987Bf3e92C9f534c0\n0x25Df024637d4e56c1aE9563987Bf3e92C9f534c1\n0x865306084235bf804c8bba8a8d56890940ca8f0b"

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// You can check headers, methods, etc. here
		if r.Header.Get("if-none-match") == "some-etag" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fmt.Fprint(w, data)
	}))
	return server
}

func TestNewCloseWithServer(t *testing.T) {
	server := newHTTPServer()
	defer server.Close()

	pool := newPoolWithParams(LIMIT, LIMIT_PER_ACCOUNT, "./", server.URL, uint64(time.Now().Unix()), &thor.NoFork)
	defer pool.Close()

	// Create a slice of transactions to be added to the pool.
	txs := make(Tx.Transactions, 0, 30)
	for i := range 15 {
		tx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])
		txs = append(txs, tx)
	}

	// Call the Fill method
	pool.Fill(txs)

	time.Sleep(1 * time.Second)
}

func FillPoolWithLegacyTxs(pool *TxPool, t *testing.T) {
	// Create a slice of transactions to be added to the pool.
	txs := make(Tx.Transactions, 0, 15)
	for range 12 {
		tx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
		txs = append(txs, tx)
	}

	// Call the Fill method
	pool.Fill(txs)

	err := pool.Add(newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(10), 100, nil, Tx.Features(0), devAccounts[0]))
	assert.Equal(t, err.Error(), "tx rejected: pool is full")
}

func FillPoolWithDynFeeTxs(pool *TxPool, t *testing.T) {
	// Advance one block to activate galactica and accept dynamic fee transactions
	addOneBlock(t, pool)

	// Create a slice of transactions to be added to the pool.
	txs := make(Tx.Transactions, 0, 15)
	for range 12 {
		tx := newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
		txs = append(txs, tx)
	}

	// Call the Fill method
	pool.Fill(txs)

	err := pool.Add(newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(10), 100, nil, Tx.Features(0), devAccounts[0]))
	assert.Equal(t, err.Error(), "tx rejected: pool is full")
	assert.Equal(t, "tx rejected: pool is full", err.Error())
}

func FillPoolWithMixedTxs(pool *TxPool, t *testing.T) {
	// Advance one block to activate galactica and accept dynamic fee transactions
	addOneBlock(t, pool)

	// Create a slice of transactions to be added to the pool.
	txs := make(Tx.Transactions, 0, 15)
	for range 6 {
		trx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
		txs = append(txs, trx)
		trx = newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
		txs = append(txs, trx)
	}

	// Call the Fill method
	pool.Fill(txs)

	err := pool.Add(newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(10), 100, nil, Tx.Features(0), devAccounts[0]))
	assert.Equal(t, err.Error(), "tx rejected: pool is full")
	assert.Equal(t, "tx rejected: pool is full", err.Error())
}

func addOneBlock(t *testing.T, pool *TxPool) {
	var sig [65]byte
	rand.Read(sig[:])

	b1 := new(block.Builder).
		ParentID(pool.repo.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(pool.Dump().RootHash()).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		Build().WithSignature(sig[:])
	if err := pool.repo.AddBlock(b1, nil, 0, true); err != nil {
		t.Fatal(err)
	}
}

func TestAddWithFullErrorUnsyncedChain(t *testing.T) {
	// First fill the pool with legacy transactions
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.NoFork)
	defer pool.Close()

	FillPoolWithLegacyTxs(pool, t)

	// Now fill the pool with dynamic fee transactions
	pool = newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.ForkConfig{GALACTICA: 1})
	FillPoolWithDynFeeTxs(pool, t)

	// Now fill the pool with mixed transactions
	pool = newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.ForkConfig{GALACTICA: 1})
	FillPoolWithMixedTxs(pool, t)
}

func TestAddWithFullErrorSyncedChain(t *testing.T) {
	pool := newPoolWithParams(LIMIT, LIMIT_PER_ACCOUNT, "./", "", uint64(time.Now().Unix()), &thor.NoFork)
	defer pool.Close()

	FillPoolWithLegacyTxs(pool, t)

	pool = newPoolWithParams(LIMIT, LIMIT_PER_ACCOUNT, "./", "", uint64(time.Now().Unix()), &thor.ForkConfig{GALACTICA: 1})
	FillPoolWithDynFeeTxs(pool, t)

	pool = newPoolWithParams(LIMIT, LIMIT_PER_ACCOUNT, "./", "", uint64(time.Now().Unix()), &thor.ForkConfig{GALACTICA: 1})
	FillPoolWithMixedTxs(pool, t)
}

func TestNewCloseWithError(t *testing.T) {
	pool := newPoolWithParams(LIMIT, LIMIT_PER_ACCOUNT, " ", " ", uint64(time.Now().Unix())+10000, &thor.NoFork)
	defer pool.Close()
}

func TestDump(t *testing.T) {
	// Create a new transaction pool with specified limits
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.ForkConfig{})
	defer pool.Close()

	// Create and add transactions to the pool
	txsToAdd := make(tx.Transactions, 0, 10)
	for i := range 5 {
		trx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])
		txsToAdd = append(txsToAdd, trx)
		assert.Nil(t, pool.Add(trx))

		trx = newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])
		txsToAdd = append(txsToAdd, trx)
		assert.Nil(t, pool.Add(trx))
	}

	// Use the Dump method to retrieve all transactions in the pool
	dumpedTxs := pool.Dump()

	// Check if the dumped transactions match the ones added
	assert.Equal(t, len(txsToAdd), len(dumpedTxs), "Number of dumped transactions should match the number added")

	// Further checks can be done to ensure that each transaction in `dumpedTxs` is also in `txsToAdd`
	for _, dumpedTx := range dumpedTxs {
		found := false
		for _, addedTx := range txsToAdd {
			if dumpedTx.ID() == addedTx.ID() {
				found = true
				break
			}
		}
		assert.True(t, found, "Dumped transaction should match one of the added transactions")
	}
}

func TestRemove(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.ForkConfig{})
	defer pool.Close()

	// Create and add a legacy transaction to the pool
	trx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	assert.Nil(t, pool.Add(trx), "Adding transaction should not produce error")

	// Ensure the transaction is in the pool
	assert.NotNil(t, pool.Get(trx.ID()), "Transaction should exist in the pool before removal")

	// Remove the transaction from the pool
	removed := pool.Remove(trx.Hash(), trx.ID())
	assert.True(t, removed, "Transaction should be successfully removed")

	// Check that the transaction is no longer in the pool
	assert.Nil(t, pool.Get(trx.ID()), "Transaction should not exist in the pool after removal")

	// Create and add a dyn fee transaction to the pool
	trx = newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	assert.Nil(t, pool.Add(trx), "Adding transaction should not produce error")

	// Ensure the transaction is in the pool
	assert.NotNil(t, pool.Get(trx.ID()), "Transaction should exist in the pool before removal")

	// Remove the transaction from the pool
	removed = pool.Remove(trx.Hash(), trx.ID())
	assert.True(t, removed, "Transaction should be successfully removed")

	// Check that the transaction is no longer in the pool
	assert.Nil(t, pool.Get(trx.ID()), "Transaction should not exist in the pool after removal")
}

func TestRemoveWithError(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.NoFork)
	defer pool.Close()

	// Create and add a transaction to the pool
	tx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	// assert.Nil(t, pool.Add(tx), "Adding transaction should not produce error")

	// Ensure the transaction is in the pool
	assert.Nil(t, pool.Get(tx.ID()), "Transaction should exist in the pool before removal")

	// Remove the transaction from the pool
	removed := pool.Remove(tx.Hash(), tx.ID())
	assert.False(t, removed, "Transaction should not be successfully removed as it doesn't exist")
}

func TestNewClose(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.NoFork)
	defer pool.Close()
}

func TestSubscribeNewTx(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.NoFork)
	defer pool.Close()

	st := pool.stater.NewState(trie.Root{Hash: pool.repo.GenesisBlock().Header().StateRoot()})
	stage, _ := st.Stage(trie.Version{Major: 1})
	root1, _ := stage.Commit()

	var sig [65]byte
	rand.Read(sig[:])

	b1 := new(block.Builder).
		ParentID(pool.repo.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(root1).
		Build().WithSignature(sig[:])
	if err := pool.repo.AddBlock(b1, nil, 0, true); err != nil {
		t.Fatal(err)
	}

	txCh := make(chan *TxEvent)

	pool.SubscribeTxEvent(txCh)

	tx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	assert.Nil(t, pool.Add(tx))

	v := true
	assert.Equal(t, &TxEvent{tx, &v}, <-txCh)
}

func TestSubscribeNewTypedTx(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.ForkConfig{})
	defer pool.Close()

	st := pool.stater.NewState(trie.Root{Hash: pool.repo.GenesisBlock().Header().StateRoot()})
	stage, _ := st.Stage(trie.Version{Major: 1})
	root1, _ := stage.Commit()

	var sig [65]byte
	rand.Read(sig[:])

	b1 := new(block.Builder).
		ParentID(pool.repo.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(root1).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		Build().WithSignature(sig[:])
	if err := pool.repo.AddBlock(b1, nil, 0, true); err != nil {
		t.Fatal(err)
	}

	txCh := make(chan *TxEvent)

	pool.SubscribeTxEvent(txCh)

	trx := tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(pool.repo.ChainTag()).
		Expiration(100).
		Gas(21000).
		MaxFeePerGas(big.NewInt(thor.InitialBaseFee * 10)).
		MaxPriorityFeePerGas(big.NewInt(100)).
		Build()
	trx = tx.MustSign(trx, devAccounts[0].PrivateKey)
	assert.Nil(t, pool.Add(trx))

	v := true
	assert.Equal(t, &TxEvent{trx, &v}, <-txCh)
}

func TestWashTxs(t *testing.T) {
	pool := newPool(1, LIMIT_PER_ACCOUNT, &thor.NoFork)
	defer pool.Close()

	txs, _, err := pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Zero(t, len(txs))
	assert.Zero(t, len(pool.Executables()))

	tx1 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	assert.Nil(t, pool.AddLocal(tx1)) // this tx won't participate in the wash out.

	txs, _, err = pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Equal(t, Tx.Transactions{tx1}, txs)

	st := pool.stater.NewState(trie.Root{Hash: pool.repo.GenesisBlock().Header().StateRoot()})
	stage, _ := st.Stage(trie.Version{Major: 1})
	root1, _ := stage.Commit()
	b1 := new(block.Builder).
		ParentID(pool.repo.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(root1).
		Build()
	pool.repo.AddBlock(b1, nil, 0, false)

	txs, _, err = pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Equal(t, Tx.Transactions{tx1}, txs)

	tx2 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[1])
	txObj2, _ := resolveTx(tx2, false)
	assert.Nil(t, pool.all.Add(txObj2, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })) // this tx will participate in the wash out.

	tx3 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[2])
	txObj3, _ := resolveTx(tx3, false)
	assert.Nil(t, pool.all.Add(txObj3, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })) // this tx will participate in the wash out.

	txs, removedCount, err := pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Equal(t, 2, len(txs))
	assert.Equal(t, 1, removedCount)
}

func TestOrderTxsAfterGalacticaFork(t *testing.T) {
	now := uint64(time.Now().Unix() - time.Now().Unix()%10 - 10)
	db := muxdb.NewMem()
	builder := genesis.NewDevnet()

	b0, _, _, err := builder.Build(state.NewStater(db))
	assert.Nil(t, err)

	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	stage, err := st.Stage(trie.Version{Major: 1})
	assert.Nil(t, err)
	root, err := stage.Commit()
	assert.Nil(t, err)

	b1 := new(block.Builder).
		ParentID(b0.Header().ID()).
		StateRoot(root).
		TotalScore(100).
		Timestamp(now + 10).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		GasLimit(thor.InitialGasLimit).
		Build()

	repo, _ := chain.NewRepository(db, b0)
	repo.AddBlock(b1, tx.Receipts{}, 0, true)

	poolLimit := 10_000
	pool := New(repo, state.NewStater(db), Options{
		Limit:           poolLimit,
		LimitPerAccount: poolLimit,
		MaxLifetime:     time.Hour,
	}, &thor.ForkConfig{GALACTICA: 1})
	defer pool.Close()

	txs := make(map[thor.Bytes32]*tx.Transaction)
	for i := range poolLimit - 2 {
		tx := tx.MustSign(generateRandomTx(t, i, repo.ChainTag()), devAccounts[i%len(devAccounts)].PrivateKey)
		txs[tx.ID()] = tx
		assert.Nil(t, pool.Add(tx))
	}

	execTxs, removed, err := pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Zero(t, removed)
	assert.Equal(t, len(txs), len(execTxs))
	assert.Equal(t, poolLimit-2, len(execTxs))
	baseGasPrice, err := builtin.Params.Native(st).Get(thor.KeyLegacyTxBaseGasPrice)
	assert.Nil(t, err)
	for i := 1; i < len(txs); i++ {
		prevGalacticaFee := galactica.GalacticaTxGasPriceAdapter(execTxs[i-1], baseGasPrice)
		currGalacticaFee := galactica.GalacticaTxGasPriceAdapter(execTxs[i], baseGasPrice)
		prevEffectiveFee := math.BigMin(new(big.Int).Sub(prevGalacticaFee.MaxFee, b1.Header().BaseFee()), prevGalacticaFee.MaxPriorityFee)
		currEffectiveFee := math.BigMin(new(big.Int).Sub(currGalacticaFee.MaxFee, b1.Header().BaseFee()), currGalacticaFee.MaxPriorityFee)
		assert.True(t, prevEffectiveFee.Cmp(currEffectiveFee) >= 0)
	}

	// Add a tx with the highest priority fee
	firstTx := tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(repo.ChainTag()).
		Expiration(100).
		Gas(21000).
		MaxFeePerGas(big.NewInt(thor.InitialBaseFee * 10)).
		MaxPriorityFeePerGas(big.NewInt(thor.InitialBaseFee * 10)).
		Build()
	firstTx = tx.MustSign(firstTx, devAccounts[0].PrivateKey)

	// Add a tx with 0 priority fee
	lastTx := tx.NewBuilder(tx.TypeDynamicFee).
		ChainTag(repo.ChainTag()).
		Expiration(100).
		Gas(21000).
		MaxFeePerGas(big.NewInt(thor.InitialBaseFee * 10)).
		MaxPriorityFeePerGas(common.Big0).
		Build()
	lastTx = tx.MustSign(lastTx, devAccounts[0].PrivateKey)

	assert.Nil(t, pool.Add(firstTx))
	assert.Nil(t, pool.Add(lastTx))

	execTxs, removed, err = pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Zero(t, removed)
	assert.Equal(t, poolLimit, len(execTxs))
	assert.Equal(t, execTxs[0].ID(), firstTx.ID())
	assert.Equal(t, execTxs[len(execTxs)-1].ID(), lastTx.ID())
}

func TestOrderTxsAfterGalacticaForkSameValues(t *testing.T) {
	now := uint64(time.Now().Unix() - time.Now().Unix()%10 - 10)
	db := muxdb.NewMem()
	builder := genesis.NewDevnet()

	b0, _, _, err := builder.Build(state.NewStater(db))
	assert.Nil(t, err)

	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	stage, err := st.Stage(trie.Version{Major: 1})
	assert.Nil(t, err)
	root, err := stage.Commit()
	assert.Nil(t, err)

	b1 := new(block.Builder).
		ParentID(b0.Header().ID()).
		StateRoot(root).
		TotalScore(100).
		Timestamp(now + 10).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		GasLimit(thor.InitialGasLimit).
		Build()

	repo, _ := chain.NewRepository(db, b0)
	repo.AddBlock(b1, tx.Receipts{}, 0, true)

	totalPoolTxs := 10_000
	pool := New(repo, state.NewStater(db), Options{
		Limit:           totalPoolTxs,
		LimitPerAccount: totalPoolTxs,
		MaxLifetime:     time.Hour,
	}, &thor.ForkConfig{GALACTICA: 1})
	defer pool.Close()

	txs := make(map[thor.Bytes32]*tx.Transaction)
	for i := range totalPoolTxs {
		tx := tx.MustSign(generateRandomTx(t, i, repo.ChainTag()), devAccounts[i%len(devAccounts)].PrivateKey)
		txs[tx.ID()] = tx
		assert.Nil(t, pool.Add(tx))
	}

	execTxs, removed, err := pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Zero(t, removed)
	assert.Equal(t, len(txs), len(execTxs))
	assert.Equal(t, totalPoolTxs, len(execTxs))
	baseGasPrice, err := builtin.Params.Native(st).Get(thor.KeyLegacyTxBaseGasPrice)
	assert.Nil(t, err)
	for i := 1; i < len(txs); i++ {
		prevGalacticaFee := galactica.GalacticaTxGasPriceAdapter(execTxs[i-1], baseGasPrice)
		currGalacticaFee := galactica.GalacticaTxGasPriceAdapter(execTxs[i], baseGasPrice)
		prevEffectiveFee := math.BigMin(new(big.Int).Sub(prevGalacticaFee.MaxFee, b1.Header().BaseFee()), prevGalacticaFee.MaxPriorityFee)
		currEffectiveFee := math.BigMin(new(big.Int).Sub(currGalacticaFee.MaxFee, b1.Header().BaseFee()), currGalacticaFee.MaxPriorityFee)
		assert.True(t, prevEffectiveFee.Cmp(currEffectiveFee) >= 0)
	}
}

func generateRandomTx(t *testing.T, seed int, chainTag byte) *tx.Transaction {
	txType := tx.TypeDynamicFee
	if (seed % 2) == 0 {
		txType = tx.TypeDynamicFee
	}

	maxFeePerGas := int64(thor.InitialBaseFee + r.IntN(thor.InitialBaseFee)) // #nosec G404
	maxPriorityFeePerGas := maxFeePerGas / int64(r.IntN(10)+1)               // #nosec G404

	trx := tx.NewBuilder(txType).
		ChainTag(chainTag).
		Expiration(100).
		Gas(21000).
		Nonce(uint64(seed)).
		MaxFeePerGas(big.NewInt(maxFeePerGas)).
		MaxPriorityFeePerGas(big.NewInt(maxPriorityFeePerGas)).
		Build()

	return trx
}

func TestFillPool(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.NoFork)
	defer pool.Close()

	// Create a slice of transactions to be added to the pool.
	txs := make(Tx.Transactions, 0, 5)
	for i := range 5 {
		tx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])
		txs = append(txs, tx)
	}

	// Call the Fill method
	pool.Fill(txs)

	// Check if the transactions are correctly added.
	// This might require accessing internal state of TxPool or using provided methods.
	for _, tx := range txs {
		assert.NotNil(t, pool.Get(tx.ID()), "Transaction should exist in the pool")
	}

	// Further checks can be made based on the behavior of your TxPool implementation.
	// For example, checking if the pool size has increased by the expected amount.
	assert.Equal(t, len(txs), pool.all.Len(), "Number of transactions in the pool should match the number added")

	// Test executables after wash
	executables, _, _ := pool.wash(pool.repo.BestBlockSummary())
	pool.executables.Store(executables)
	assert.Equal(t, len(txs), len(pool.Executables()), "Number of transactions in the pool should match the number added")
}

func TestFillPoolWithMixedTxs(t *testing.T) {
	now := uint64(time.Now().Unix() - time.Now().Unix()%10 - 10)
	db := muxdb.NewMem()
	builder := genesis.NewDevnet()

	b0, _, _, err := builder.Build(state.NewStater(db))
	assert.Nil(t, err)

	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	stage, err := st.Stage(trie.Version{Major: 1})
	assert.Nil(t, err)
	root, err := stage.Commit()
	assert.Nil(t, err)

	b1 := new(block.Builder).
		ParentID(b0.Header().ID()).
		StateRoot(root).
		TotalScore(100).
		Timestamp(now + 10).
		BaseFee(thor.InitialBaseGasPrice).
		GasLimit(thor.InitialGasLimit).
		Build()

	repo, _ := chain.NewRepository(db, b0)
	repo.AddBlock(b1, tx.Receipts{}, 0, true)
	pool := New(repo, state.NewStater(db), Options{
		Limit:           LIMIT,
		LimitPerAccount: LIMIT_PER_ACCOUNT,
		MaxLifetime:     time.Hour,
	}, &thor.ForkConfig{GALACTICA: 0})
	defer pool.Close()

	// Create a slice of transactions to be added to the pool.
	txs := make(Tx.Transactions, 0, 10)
	for i := range 5 {
		tr := newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])
		txs = append(txs, tr)

		tr = newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])
		txs = append(txs, tr)
	}

	// Call the Fill method
	pool.Fill(txs)

	// Check if the transactions are correctly added.
	// This might require accessing internal state of TxPool or using provided methods.
	for _, tx := range txs {
		assert.NotNil(t, pool.Get(tx.ID()), "Transaction should exist in the pool")
	}

	// Further checks can be made based on the behavior of your TxPool implementation.
	// For example, checking if the pool size has increased by the expected amount.
	assert.Equal(t, len(txs), pool.all.Len(), "Number of transactions in the pool should match the number added")

	// Test executables after wash
	executables, _, _ := pool.wash(pool.repo.BestBlockSummary())
	pool.executables.Store(executables)
	assert.Equal(t, len(txs), len(pool.Executables()), "Number of transactions in the pool should match the number added")
}

func TestAdd(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.ForkConfig{})
	defer pool.Close()
	st := pool.stater.NewState(trie.Root{Hash: pool.repo.GenesisBlock().Header().StateRoot()})
	stage, _ := st.Stage(trie.Version{Major: 1})
	root1, _ := stage.Commit()

	var sig [65]byte
	rand.Read(sig[:])
	b1 := new(block.Builder).
		ParentID(pool.repo.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(root1).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		Build().WithSignature(sig[:])
	pool.repo.AddBlock(b1, nil, 0, true)
	acc := devAccounts[0]

	dupTx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)

	tests := []struct {
		tx     *tx.Transaction
		errStr string
	}{
		{newTx(tx.TypeLegacy, pool.repo.ChainTag()+1, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc), "bad tx: chain tag mismatch"},
		{newTx(tx.TypeDynamicFee, pool.repo.ChainTag()+1, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc), "bad tx: chain tag mismatch"},
		{dupTx, ""},
		{dupTx, ""},
	}

	for _, tt := range tests {
		err := pool.Add(tt.tx)
		if tt.errStr == "" {
			assert.Nil(t, err)
		} else {
			assert.Equal(t, tt.errStr, err.Error())
		}
	}

	raw, _ := hex.DecodeString(fmt.Sprintf("f8dc81%v84aabbccdd20f840df947567d83b7b8d80addcb281a71d54fc7b3364ffed82271086000000606060df947567d83b7b8d80addcb281a71d54fc7b3364ffed824e20860000006060608180830334508083bc614ec20108b88256e32450c1907f627d2c11fe5a9d0216be1712f4938b5feb04e37edef236c56266c3378acf97994beff22698b70023f486645d29cb23b479a7b044f7c6b104d2000584fcb3964446d4d832dcc849e2d76ea7e04a4ebdc3a4b61e7997e93277363d4e7fe9315e7f6dd8d9c0a8bff5879503f5c04adab8b08772499e74d34f67923501",
		hex.EncodeToString([]byte{pool.repo.ChainTag()}),
	))
	badReserved := new(tx.Transaction)
	if err := badReserved.UnmarshalBinary(raw); err != nil {
		t.Error(err)
	}

	var data [64 * 1024]byte
	rand.Read(data[:])

	tests = []struct {
		tx     *Tx.Transaction
		errStr string
	}{
		{newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(10), 100, nil, Tx.Features(0), acc), "tx rejected: tx is not executable"},
		{newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(100), 100, nil, Tx.Features(0), acc), "tx rejected: block ref out of schedule"},
		{newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(0), acc), "tx rejected: tx is not executable"},
		{newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(2), acc), "tx rejected: unsupported features"},
		{newTx(tx.TypeLegacy, pool.repo.ChainTag(), []*tx.Clause{tx.NewClause(nil).WithData(data[:])}, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(0), acc), "tx rejected: size too large"},
		{badReserved, "tx rejected: unsupported features"},
		{newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(10), 100, nil, Tx.Features(0), acc), "tx rejected: tx is not executable"},
		{newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(100), 100, nil, Tx.Features(0), acc), "tx rejected: block ref out of schedule"},
		{newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(0), acc), "tx rejected: tx is not executable"},
		{newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(2), acc), "tx rejected: unsupported features"},
		{newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), []*tx.Clause{tx.NewClause(nil).WithData(data[:])}, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(0), acc), "tx rejected: size too large"},
	}

	for _, tt := range tests {
		err := pool.StrictlyAdd(tt.tx)
		if tt.errStr == "" {
			assert.Nil(t, err)
		} else {
			assert.Equal(t, tt.errStr, err.Error())
		}
	}
}

func TestBeforeVIP191Add(t *testing.T) {
	db := muxdb.NewMem()
	defer db.Close()

	chain := newChainRepo(db)

	acc := devAccounts[0]

	pool := New(chain, state.NewStater(db), Options{
		Limit:           10,
		LimitPerAccount: 2,
		MaxLifetime:     time.Hour,
	}, &thor.NoFork)
	defer pool.Close()

	err := pool.StrictlyAdd(newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(200), 100, nil, Tx.Features(1), acc))

	assert.Equal(t, "tx rejected: unsupported features", err.Error())
}

func TestPoolLimit(t *testing.T) {
	// synced
	pool := newPoolWithParams(2, 1, "", "", uint64(time.Now().Unix()), &thor.NoFork)
	defer pool.Close()

	trx1 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	trx2 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	pool.add(trx1, false, false)

	err := pool.add(trx2, false, false)
	assert.Equal(t, "tx rejected: account quota exceeded", err.Error())

	// not synced
	pool = newPool(2, 1, &thor.NoFork)

	trx1 = newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	trx2 = newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	pool.add(trx1, false, false)
	err = pool.add(trx2, false, false)
	assert.Equal(t, "tx rejected: account quota exceeded", err.Error())
}

func TestExecutableAndNonExecutableLimits(t *testing.T) {
	// executable pool limit
	pool := newPoolWithParams(10, 2, "", "", uint64(time.Now().Unix()), &thor.NoFork)
	defer pool.Close()

	// Create a slice of transactions to be added to the pool.
	txs := make(Tx.Transactions, 0, 11)
	for i := range 12 {
		tx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])
		pool.add(tx, false, false)
		txs = append(txs, tx)
	}
	pool.executables.Store(txs)

	trx1 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[1])

	err := pool.add(trx1, false, false)
	assert.Equal(t, "tx rejected: pool is full", err.Error())

	trx2 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, tx.Features(0), devAccounts[1])

	err = pool.add(trx2, false, false)
	assert.Equal(t, "tx rejected: pool is full", err.Error())

	// non-executable pool limit
	pool = newPoolWithParams(5, 2, "", "", uint64(time.Now().Unix()), &thor.NoFork)
	defer pool.Close()

	trx1 = newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, tx.Features(0), devAccounts[0])

	err = pool.add(trx1, false, false)
	assert.Nil(t, err)

	// dependant fails
	trx2 = newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, tx.Features(0), devAccounts[2])

	err = pool.add(trx2, false, false)

	assert.Equal(t, "tx rejected: non executable pool is full", err.Error())

	// higher block fails
	trx2 = newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(tx.BlockRef{}.Number()+2), 100, nil, tx.Features(0), devAccounts[2])

	err = pool.add(trx2, false, false)

	assert.Equal(t, "tx rejected: non executable pool is full", err.Error())
}

func TestNonExecutables(t *testing.T) {
	pool := newPoolWithParams(100, 100, "", "", uint64(time.Now().Unix()), &thor.NoFork)

	// loop 90 times
	for i := range 90 {
		assert.NoError(t, pool.AddLocal(newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])))
	}

	executables, _, _ := pool.wash(pool.repo.BestBlockSummary())
	pool.executables.Store(executables)

	// add 1 non-executable
	assert.NoError(t, pool.AddLocal(newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, tx.Features(0), devAccounts[2])))
}

func TestExpiredTxs(t *testing.T) {
	pool := newPoolWithMaxLifetime(100, 100, "", "", uint64(time.Now().Unix()), 3*time.Second, &thor.NoFork)

	// loop 90 times
	for i := range 90 {
		assert.NoError(t, pool.Add(newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])))
	}

	executables, _, _ := pool.wash(pool.repo.BestBlockSummary())
	pool.executables.Store(executables)

	// add 1 non-executable
	assert.NoError(t, pool.Add(newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, tx.Features(0), devAccounts[2])))

	executables, washed, err := pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Equal(t, 90, len(executables))
	assert.Equal(t, 0, washed)
	assert.Equal(t, 91, pool.all.Len())

	time.Sleep(3 * time.Second)
	executables, washed, err = pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Equal(t, 0, len(executables))
	assert.Equal(t, 91, washed)
	assert.Equal(t, 0, pool.all.Len())
}

func TestBlocked(t *testing.T) {
	acc := devAccounts[len(devAccounts)-1]

	file, err := os.CreateTemp("", "blocklist*")
	assert.Nil(t, err)
	file.WriteString(acc.Address.String())
	file.Close()

	pool := newPoolWithParams(LIMIT, LIMIT_PER_ACCOUNT, file.Name(), "", uint64(time.Now().Unix()), &thor.NoFork)
	defer pool.Close()
	<-time.After(10 * time.Millisecond)

	// adding blocked should return nil
	trx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[len(devAccounts)-1])
	err = pool.Add(trx)
	assert.Nil(t, err)

	// added into all, will be washed out
	txObj, err := resolveTx(trx, false)
	assert.Nil(t, err)
	pool.all.Add(txObj, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })

	pool.wash(pool.repo.BestBlockSummary())
	got := pool.Get(trx.ID())
	assert.Nil(t, got)

	os.Remove(file.Name())
}

func TestWash(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.NoFork)
	defer pool.Close()

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"MaxLife", func(t *testing.T) {
				trx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[len(devAccounts)-1])
				pool.add(trx, false, false)

				txObj := pool.all.mapByID[trx.ID()]
				txObj.timeAdded = txObj.timeAdded - int64(pool.options.MaxLifetime)*2

				pool.wash(pool.repo.BestBlockSummary())
				got := pool.Get(trx.ID())
				assert.Nil(t, got)
			},
		},
		{
			"Not enough VTHO", func(t *testing.T) {
				priv, err := crypto.GenerateKey()
				assert.Nil(t, err)

				acc := genesis.DevAccount{
					Address:    thor.Address(crypto.PubkeyToAddress(priv.PublicKey)),
					PrivateKey: priv,
				}

				trx := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)

				txObj, err := resolveTx(trx, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })

				pool.wash(pool.repo.BestBlockSummary())
				got := pool.Get(trx.ID())
				assert.Nil(t, got)
			},
		},
		{
			"Future tx", func(t *testing.T) {
				pool := newPool(1, LIMIT_PER_ACCOUNT, &thor.NoFork)
				defer pool.Close()

				priv, err := crypto.GenerateKey()
				assert.Nil(t, err)

				acc := genesis.DevAccount{
					Address:    thor.Address(crypto.PubkeyToAddress(priv.PublicKey)),
					PrivateKey: priv,
				}

				trx1 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
				trx2 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
				trx3 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(pool.repo.BestBlockSummary().Header.Number()+10), 100, nil, tx.Features(0), acc)
				pool.add(trx1, false, false)

				txObj, err := resolveTx(trx2, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })

				txObj, err = resolveTx(trx3, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })

				pool.wash(pool.repo.BestBlockSummary())
				got := pool.Get(trx3.ID())
				assert.Nil(t, got)
			},
		},
		{
			"Executable + Non executable beyond limit", func(t *testing.T) {
				pool := newPool(1, LIMIT_PER_ACCOUNT, &thor.NoFork)
				defer pool.Close()

				priv, err := crypto.GenerateKey()
				assert.Nil(t, err)

				acc := genesis.DevAccount{
					Address:    thor.Address(crypto.PubkeyToAddress(priv.PublicKey)),
					PrivateKey: priv,
				}

				trx1 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
				trx2 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(pool.repo.BestBlockSummary().Header.Number()+10), 100, nil, tx.Features(0), devAccounts[0])
				trx3 := newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(pool.repo.BestBlockSummary().Header.Number()+10), 100, nil, tx.Features(0), acc)
				pool.add(trx1, false, false)

				txObj, err := resolveTx(trx2, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })

				txObj, err = resolveTx(trx3, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })

				pool.wash(pool.repo.BestBlockSummary())
				// all non executable should be washed out
				got := pool.Get(trx2.ID())
				assert.Nil(t, got)
				got = pool.Get(trx3.ID())
				assert.Nil(t, got)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestWashWithDynFeeTx(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT, &thor.ForkConfig{GALACTICA: 0})
	defer pool.Close()

	st := pool.stater.NewState(trie.Root{Hash: pool.repo.GenesisBlock().Header().StateRoot()})
	stage, _ := st.Stage(trie.Version{Major: 1})
	root1, _ := stage.Commit()

	var sig [65]byte
	rand.Read(sig[:])

	b1 := new(block.Builder).
		ParentID(pool.repo.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(root1).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		Build().WithSignature(sig[:])
	if err := pool.repo.AddBlock(b1, nil, 0, true); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"MaxLife with dynFeeTx", func(t *testing.T) {
				trx := newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[len(devAccounts)-1])
				pool.add(trx, false, false)

				txObj := pool.all.mapByID[trx.ID()]
				txObj.timeAdded = txObj.timeAdded - int64(pool.options.MaxLifetime)*2

				pool.wash(pool.repo.BestBlockSummary())
				got := pool.Get(trx.ID())
				assert.Nil(t, got)
			},
		},
		{
			"Not enough VTHO with dynFeeTx", func(t *testing.T) {
				priv, err := crypto.GenerateKey()
				assert.Nil(t, err)

				acc := genesis.DevAccount{
					Address:    thor.Address(crypto.PubkeyToAddress(priv.PublicKey)),
					PrivateKey: priv,
				}

				trx := newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)

				txObj, err := resolveTx(trx, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })

				pool.wash(pool.repo.BestBlockSummary())
				got := pool.Get(trx.ID())
				assert.Nil(t, got)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestWashWithDynFeeTxAndPoolLimit(t *testing.T) {
	pool := newPool(1, LIMIT_PER_ACCOUNT, &thor.ForkConfig{GALACTICA: 0})
	defer pool.Close()

	st := pool.stater.NewState(trie.Root{Hash: pool.repo.GenesisBlock().Header().StateRoot()})
	stage, _ := st.Stage(trie.Version{Major: 1})
	root1, _ := stage.Commit()

	var sig [65]byte
	rand.Read(sig[:])

	b1 := new(block.Builder).
		ParentID(pool.repo.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(root1).
		BaseFee(thor.InitialBaseGasPrice).
		Build().WithSignature(sig[:])
	if err := pool.repo.AddBlock(b1, nil, 0, true); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"Future tx with dynFeeTx", func(t *testing.T) {
				priv, err := crypto.GenerateKey()
				assert.Nil(t, err)

				acc := genesis.DevAccount{
					Address:    thor.Address(crypto.PubkeyToAddress(priv.PublicKey)),
					PrivateKey: priv,
				}

				trx1 := newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
				trx2 := newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
				trx3 := newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(pool.repo.BestBlockSummary().Header.Number()+10), 100, nil, tx.Features(0), acc)
				pool.add(trx1, false, false)

				txObj, err := resolveTx(trx2, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })

				txObj, err = resolveTx(trx3, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })

				pool.wash(pool.repo.BestBlockSummary())
				got := pool.Get(trx3.ID())
				assert.Nil(t, got)
			},
		},
		{
			"Executable + Non executable beyond limit with dynFeeTx", func(t *testing.T) {
				priv, err := crypto.GenerateKey()
				assert.Nil(t, err)

				acc := genesis.DevAccount{
					Address:    thor.Address(crypto.PubkeyToAddress(priv.PublicKey)),
					PrivateKey: priv,
				}

				trx1 := newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
				trx2 := newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(pool.repo.BestBlockSummary().Header.Number()+10), 100, nil, tx.Features(0), devAccounts[0])
				trx3 := newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(pool.repo.BestBlockSummary().Header.Number()+10), 100, nil, tx.Features(0), acc)
				pool.add(trx1, false, false)

				txObj, err := resolveTx(trx2, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })

				txObj, err = resolveTx(trx3, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT, func(_ thor.Address, _ *big.Int) error { return nil })

				pool.wash(pool.repo.BestBlockSummary())
				// all non executable should be washed out
				got := pool.Get(trx2.ID())
				assert.Nil(t, got)
				got = pool.Get(trx3.ID())
				assert.Nil(t, got)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestAddOverPendingCost(t *testing.T) {
	now := uint64(time.Now().Unix() - time.Now().Unix()%10 - 10)
	db := muxdb.NewMem()
	builder := new(genesis.Builder).
		GasLimit(thor.InitialGasLimit).
		Timestamp(now).
		State(func(state *state.State) error {
			if err := state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes()); err != nil {
				return err
			}
			bal, _ := new(big.Int).SetString("42000000000000000000", 10)
			for _, acc := range devAccounts {
				state.SetEnergy(acc.Address, bal, now)
			}
			return nil
		})

	method, found := builtin.Params.ABI.MethodByName("set")
	assert.True(t, found)

	var executor thor.Address
	data, err := method.EncodeInput(thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
	assert.Nil(t, err)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), thor.Address{})

	data, err = method.EncodeInput(thor.KeyLegacyTxBaseGasPrice, thor.InitialBaseGasPrice)
	assert.Nil(t, err)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	b0, _, _, err := builder.Build(state.NewStater(db))
	assert.Nil(t, err)

	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	stage, err := st.Stage(trie.Version{Major: 1})
	assert.Nil(t, err)
	root, err := stage.Commit()
	assert.Nil(t, err)

	var feat tx.Features
	feat.SetDelegated(true)
	b1 := new(block.Builder).
		ParentID(b0.Header().ID()).
		StateRoot(root).
		TotalScore(100).
		Timestamp(now + 10).
		GasLimit(thor.InitialGasLimit).
		TransactionFeatures(feat).Build()

	repo, _ := chain.NewRepository(db, b0)
	repo.AddBlock(b1, tx.Receipts{}, 0, true)
	pool := New(repo, state.NewStater(db), Options{
		Limit:           LIMIT,
		LimitPerAccount: LIMIT,
		MaxLifetime:     time.Hour,
	}, &thor.NoFork)
	defer pool.Close()

	// first and second tx should be fine
	err = pool.Add(newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0]))
	assert.Nil(t, err)
	err = pool.Add(newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0]))
	assert.Nil(t, err)
	// third tx should be rejected due to insufficient energy
	err = pool.Add(newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
	err = pool.Add(newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
	// delegated fee should also be counted
	err = pool.Add(newDelegatedTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, devAccounts[9], devAccounts[0]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
	err = pool.Add(newDelegatedTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, devAccounts[9], devAccounts[0]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")

	// first and second tx should be fine
	err = pool.Add(newDelegatedTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, devAccounts[1], devAccounts[2]))
	assert.Nil(t, err)
	err = pool.Add(newTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[2]))
	assert.Nil(t, err)
	// delegated fee should also be counted
	err = pool.Add(newDelegatedTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, devAccounts[8], devAccounts[2]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
	err = pool.Add(newDelegatedTx(tx.TypeLegacy, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, devAccounts[8], devAccounts[2]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
}

func TestAddOverPendingCostDynamicFee(t *testing.T) {
	now := uint64(time.Now().Unix() - time.Now().Unix()%10 - 10)
	db := muxdb.NewMem()
	builder := new(genesis.Builder).
		GasLimit(thor.InitialGasLimit).
		Timestamp(now).
		State(func(state *state.State) error {
			if err := state.SetCode(builtin.Params.Address, builtin.Params.RuntimeBytecodes()); err != nil {
				return err
			}
			if err := state.SetCode(builtin.Prototype.Address, builtin.Prototype.RuntimeBytecodes()); err != nil {
				return err
			}
			bal, _ := new(big.Int).SetString("420000000000000000", 10)
			for _, acc := range devAccounts {
				state.SetEnergy(acc.Address, bal, now)
			}
			return nil
		})

	method, found := builtin.Params.ABI.MethodByName("set")
	assert.True(t, found)

	var executor thor.Address
	data, err := method.EncodeInput(thor.KeyExecutorAddress, new(big.Int).SetBytes(executor[:]))
	assert.Nil(t, err)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), thor.Address{})

	data, err = method.EncodeInput(thor.KeyLegacyTxBaseGasPrice, thor.InitialBaseGasPrice)
	assert.Nil(t, err)
	builder.Call(tx.NewClause(&builtin.Params.Address).WithData(data), executor)

	b0, _, _, err := builder.Build(state.NewStater(db))
	assert.Nil(t, err)

	st := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})
	stage, err := st.Stage(trie.Version{Major: 1})
	assert.Nil(t, err)
	root, err := stage.Commit()
	assert.Nil(t, err)

	var feat tx.Features
	feat.SetDelegated(true)
	b1 := new(block.Builder).
		ParentID(b0.Header().ID()).
		StateRoot(root).
		TotalScore(100).
		Timestamp(now + 10).
		BaseFee(big.NewInt(thor.InitialBaseFee)).
		GasLimit(thor.InitialGasLimit).
		TransactionFeatures(feat).Build()

	repo, _ := chain.NewRepository(db, b0)
	repo.AddBlock(b1, tx.Receipts{}, 0, true)
	pool := New(repo, state.NewStater(db), Options{
		Limit:           LIMIT,
		LimitPerAccount: LIMIT,
		MaxLifetime:     time.Hour,
	}, &thor.ForkConfig{GALACTICA: 0})
	defer pool.Close()

	// first and second tx should be fine
	err = pool.Add(newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0]))
	assert.Nil(t, err)
	err = pool.Add(newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0]))
	assert.Nil(t, err)
	// third tx should be rejected due to insufficient energy
	err = pool.Add(newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
	err = pool.Add(newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
	// delegated fee should also be counted
	err = pool.Add(newDelegatedTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, devAccounts[9], devAccounts[0]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
	err = pool.Add(newDelegatedTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, devAccounts[9], devAccounts[0]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")

	// first and second tx should be fine
	err = pool.Add(newDelegatedTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, devAccounts[1], devAccounts[2]))
	assert.Nil(t, err)
	err = pool.Add(newTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[2]))
	assert.Nil(t, err)
	// delegated fee should also be counted
	err = pool.Add(newDelegatedTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, devAccounts[8], devAccounts[2]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
	err = pool.Add(newDelegatedTx(tx.TypeDynamicFee, pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, devAccounts[8], devAccounts[2]))
	assert.EqualError(t, err, "tx rejected: insufficient energy for overall pending cost")
}

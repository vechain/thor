// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package txpool

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	Tx "github.com/vechain/thor/v2/tx"
)

const LIMIT = 10
const LIMIT_PER_ACCOUNT = 2

var devAccounts = genesis.DevAccounts()

func newPool(limit int, limitPerAccount int) *TxPool {
	db := muxdb.NewMem()
	repo := newChainRepo(db)
	return New(repo, state.NewStater(db), Options{
		Limit:           limit,
		LimitPerAccount: limitPerAccount,
		MaxLifetime:     time.Hour,
	})
}

func newPoolWithParams(limit int, limitPerAccount int, BlocklistCacheFilePath string, BlocklistFetchURL string, timestamp uint64) *TxPool {
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
		MaxLifetime:            time.Hour,
		BlocklistCacheFilePath: BlocklistCacheFilePath,
		BlocklistFetchURL:      BlocklistFetchURL,
	})
}

func newHttpServer() *httptest.Server {
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
	server := newHttpServer()
	defer server.Close()

	pool := newPoolWithParams(LIMIT, LIMIT_PER_ACCOUNT, "./", server.URL, uint64(time.Now().Unix()))
	defer pool.Close()

	// Create a slice of transactions to be added to the pool.
	txs := make(Tx.Transactions, 0, 15)
	for i := 0; i < 15; i++ {
		tx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])
		txs = append(txs, tx)
	}

	// Call the Fill method
	pool.Fill(txs)

	time.Sleep(1 * time.Second)
}

func FillPoolWithTxs(pool *TxPool, t *testing.T) {
	// Create a slice of transactions to be added to the pool.
	txs := make(Tx.Transactions, 0, 15)
	for i := 0; i < 12; i++ {
		tx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
		txs = append(txs, tx)
	}

	// Call the Fill method
	pool.Fill(txs)

	err := pool.Add(newTx(pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(10), 100, nil, Tx.Features(0), devAccounts[0]))
	assert.Equal(t, err.Error(), "tx rejected: pool is full")
}

func TestAddWithFullErrorUnsyncedChain(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT)
	defer pool.Close()

	FillPoolWithTxs(pool, t)
}

func TestAddWithFullErrorSyncedChain(t *testing.T) {
	pool := newPoolWithParams(LIMIT, LIMIT_PER_ACCOUNT, "./", "", uint64(time.Now().Unix()))
	defer pool.Close()

	FillPoolWithTxs(pool, t)
}

func TestNewCloseWithError(t *testing.T) {
	pool := newPoolWithParams(LIMIT, LIMIT_PER_ACCOUNT, " ", " ", uint64(time.Now().Unix())+10000)
	defer pool.Close()
}

func TestDump(t *testing.T) {
	// Create a new transaction pool with specified limits
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT)
	defer pool.Close()

	// Create and add transactions to the pool
	txsToAdd := make(tx.Transactions, 0, 5)
	for i := 0; i < 5; i++ {
		tx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])
		txsToAdd = append(txsToAdd, tx)
		assert.Nil(t, pool.Add(tx))
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
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT)
	defer pool.Close()

	// Create and add a transaction to the pool
	tx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	assert.Nil(t, pool.Add(tx), "Adding transaction should not produce error")

	// Ensure the transaction is in the pool
	assert.NotNil(t, pool.Get(tx.ID()), "Transaction should exist in the pool before removal")

	// Remove the transaction from the pool
	removed := pool.Remove(tx.Hash(), tx.ID())
	assert.True(t, removed, "Transaction should be successfully removed")

	// Check that the transaction is no longer in the pool
	assert.Nil(t, pool.Get(tx.ID()), "Transaction should not exist in the pool after removal")
}

func TestRemoveWithError(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT)
	defer pool.Close()

	// Create and add a transaction to the pool
	tx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	// assert.Nil(t, pool.Add(tx), "Adding transaction should not produce error")

	// Ensure the transaction is in the pool
	assert.Nil(t, pool.Get(tx.ID()), "Transaction should exist in the pool before removal")

	// Remove the transaction from the pool
	removed := pool.Remove(tx.Hash(), tx.ID())
	assert.False(t, removed, "Transaction should not be successfully removed as it doesn't exist")
}

func TestNewClose(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT)
	defer pool.Close()
}

func TestSubscribeNewTx(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT)
	defer pool.Close()

	st := pool.stater.NewState(pool.repo.GenesisBlock().Header().StateRoot(), 0, 0, 0)
	stage, _ := st.Stage(1, 0)
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
	if err := pool.repo.AddBlock(b1, nil, 0); err != nil {
		t.Fatal(err)
	}
	pool.repo.SetBestBlockID(b1.Header().ID())

	txCh := make(chan *TxEvent)

	pool.SubscribeTxEvent(txCh)

	tx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	assert.Nil(t, pool.Add(tx))

	v := true
	assert.Equal(t, &TxEvent{tx, &v}, <-txCh)
}

func TestWashTxs(t *testing.T) {
	pool := newPool(1, LIMIT_PER_ACCOUNT)
	defer pool.Close()

	txs, _, err := pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Zero(t, len(txs))
	assert.Zero(t, len(pool.Executables()))

	tx1 := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	assert.Nil(t, pool.AddLocal(tx1)) // this tx won't participate in the wash out.

	txs, _, err = pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Equal(t, Tx.Transactions{tx1}, txs)

	st := pool.stater.NewState(pool.repo.GenesisBlock().Header().StateRoot(), 0, 0, 0)
	stage, _ := st.Stage(1, 0)
	root1, _ := stage.Commit()
	b1 := new(block.Builder).
		ParentID(pool.repo.GenesisBlock().Header().ID()).
		Timestamp(uint64(time.Now().Unix())).
		TotalScore(100).
		GasLimit(10000000).
		StateRoot(root1).
		Build()
	pool.repo.AddBlock(b1, nil, 0)

	txs, _, err = pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Equal(t, Tx.Transactions{tx1}, txs)

	tx2 := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[1])
	txObj2, _ := resolveTx(tx2, false)
	assert.Nil(t, pool.all.Add(txObj2, LIMIT_PER_ACCOUNT)) // this tx will participate in the wash out.

	tx3 := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[2])
	txObj3, _ := resolveTx(tx3, false)
	assert.Nil(t, pool.all.Add(txObj3, LIMIT_PER_ACCOUNT)) // this tx will participate in the wash out.

	txs, removedCount, err := pool.wash(pool.repo.BestBlockSummary())
	assert.Nil(t, err)
	assert.Equal(t, 2, len(txs))
	assert.Equal(t, 1, removedCount)
}

func TestFillPool(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT)
	defer pool.Close()

	// Create a slice of transactions to be added to the pool.
	txs := make(Tx.Transactions, 0, 5)
	for i := 0; i < 5; i++ {
		tx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[i%len(devAccounts)])
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

func TestAdd(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT)
	defer pool.Close()
	st := pool.stater.NewState(pool.repo.GenesisBlock().Header().StateRoot(), 0, 0, 0)
	stage, _ := st.Stage(1, 0)
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
	pool.repo.AddBlock(b1, nil, 0)
	pool.repo.SetBestBlockID(b1.Header().ID())
	acc := devAccounts[0]

	dupTx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)

	tests := []struct {
		tx     *tx.Transaction
		errStr string
	}{
		{newTx(pool.repo.ChainTag()+1, nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc), "bad tx: chain tag mismatch"},
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
	var badReserved *Tx.Transaction
	if err := rlp.DecodeBytes(raw, &badReserved); err != nil {
		t.Error(err)
	}

	var data [64 * 1024]byte
	rand.Read(data[:])

	tests = []struct {
		tx     *Tx.Transaction
		errStr string
	}{
		{newTx(pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(10), 100, nil, Tx.Features(0), acc), "tx rejected: tx is not executable"},
		{newTx(pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(100), 100, nil, Tx.Features(0), acc), "tx rejected: block ref out of schedule"},
		{newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(0), acc), "tx rejected: tx is not executable"},
		{newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(2), acc), "tx rejected: unsupported features"},
		{newTx(pool.repo.ChainTag(), []*tx.Clause{tx.NewClause(nil).WithData(data[:])}, 21000, tx.BlockRef{}, 100, &thor.Bytes32{1}, Tx.Features(0), acc), "tx rejected: size too large"},
		{badReserved, "tx rejected: unsupported features"},
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
	})
	defer pool.Close()

	err := pool.StrictlyAdd(newTx(pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(200), 100, nil, Tx.Features(1), acc))

	assert.Equal(t, "tx rejected: unsupported features", err.Error())
}

func TestPoolLimit(t *testing.T) {
	// synced
	pool := newPoolWithParams(2, 1, "", "", uint64(time.Now().Unix()))
	defer pool.Close()

	trx1 := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	trx2 := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	pool.add(trx1, false, false)

	err := pool.add(trx2, false, false)
	assert.Equal(t, "tx rejected: account quota exceeded", err.Error())

	// not synced
	pool = newPool(2, 1)

	trx1 = newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	trx2 = newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
	pool.add(trx1, false, false)
	err = pool.add(trx2, false, false)
	assert.Equal(t, "tx rejected: account quota exceeded", err.Error())
}

func TestBlocked(t *testing.T) {
	acc := devAccounts[len(devAccounts)-1]

	file, err := os.CreateTemp("", "blocklist*")
	assert.Nil(t, err)
	file.WriteString(acc.Address.String())
	file.Close()

	pool := newPoolWithParams(LIMIT, LIMIT_PER_ACCOUNT, file.Name(), "", uint64(time.Now().Unix()))
	defer pool.Close()
	<-time.After(10 * time.Millisecond)

	// adding blocked should return nil
	trx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[len(devAccounts)-1])
	err = pool.Add(trx)
	assert.Nil(t, err)

	// added into all, will be washed out
	txObj, err := resolveTx(trx, false)
	assert.Nil(t, err)
	pool.all.Add(txObj, LIMIT_PER_ACCOUNT)

	pool.wash(pool.repo.BestBlockSummary())
	got := pool.Get(trx.ID())
	assert.Nil(t, got)

	os.Remove(file.Name())
}

func TestWash(t *testing.T) {
	pool := newPool(LIMIT, LIMIT_PER_ACCOUNT)
	defer pool.Close()

	tests := []struct {
		name     string
		testFunc func(*testing.T)
	}{
		{
			"MaxLife", func(t *testing.T) {
				trx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[len(devAccounts)-1])
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

				trx := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), acc)

				txObj, err := resolveTx(trx, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT)

				pool.wash(pool.repo.BestBlockSummary())
				got := pool.Get(trx.ID())
				assert.Nil(t, got)
			},
		},
		{
			"Future tx", func(t *testing.T) {
				pool := newPool(1, LIMIT_PER_ACCOUNT)
				defer pool.Close()

				priv, err := crypto.GenerateKey()
				assert.Nil(t, err)

				acc := genesis.DevAccount{
					Address:    thor.Address(crypto.PubkeyToAddress(priv.PublicKey)),
					PrivateKey: priv,
				}

				trx1 := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
				trx2 := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
				trx3 := newTx(pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(pool.repo.BestBlockSummary().Header.Number()+10), 100, nil, tx.Features(0), acc)
				pool.add(trx1, false, false)

				txObj, err := resolveTx(trx2, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT)

				txObj, err = resolveTx(trx3, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT)

				pool.wash(pool.repo.BestBlockSummary())
				got := pool.Get(trx3.ID())
				assert.Nil(t, got)
			},
		},
		{
			"Executable + Non executable beyond limit", func(t *testing.T) {
				pool := newPool(1, LIMIT_PER_ACCOUNT)
				defer pool.Close()

				priv, err := crypto.GenerateKey()
				assert.Nil(t, err)

				acc := genesis.DevAccount{
					Address:    thor.Address(crypto.PubkeyToAddress(priv.PublicKey)),
					PrivateKey: priv,
				}

				trx1 := newTx(pool.repo.ChainTag(), nil, 21000, tx.BlockRef{}, 100, nil, tx.Features(0), devAccounts[0])
				trx2 := newTx(pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(pool.repo.BestBlockSummary().Header.Number()+10), 100, nil, tx.Features(0), devAccounts[0])
				trx3 := newTx(pool.repo.ChainTag(), nil, 21000, tx.NewBlockRef(pool.repo.BestBlockSummary().Header.Number()+10), 100, nil, tx.Features(0), acc)
				pool.add(trx1, false, false)

				txObj, err := resolveTx(trx2, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT)

				txObj, err = resolveTx(trx3, false)
				assert.Nil(t, err)
				pool.all.Add(txObj, LIMIT_PER_ACCOUNT)

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

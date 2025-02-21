// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"path/filepath"
	"runtime/debug"
	"sync"
	"testing"
	"time"

	"github.com/elastic/gosigar"
	"github.com/ethereum/go-ethereum/common/fdlimit"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

var (
	cachedAccounts    []genesis.DevAccount
	once              sync.Once
	blockCount        = 1_000
	createManyTxFuncs = []func(signerPK *ecdsa.PrivateKey, thorChain *testchain.Chain) (tx.Transactions, error){
		createManyClausesPerTxLegacy,
		createManyClausesPerTxDynFee,
	}
)

func getCachedAccounts(b *testing.B) []genesis.DevAccount {
	once.Do(func() {
		now := time.Now()
		cachedAccounts = createAccounts(b, 10_000)
		b.Logf("Created accounts in: %f secs", time.Since(now).Seconds())
	})
	return cachedAccounts
}

func BenchmarkFetchTx_RealDB_RandomSigners_ManyClausesPerTx(b *testing.B) {
	// create state accounts
	accounts := getCachedAccounts(b)

	for _, createManyClausesPerTx := range createManyTxFuncs { // randomly pick a signer for signing the transactions
		randomSignerFunc := randomPickSignerFunc(accounts, createManyClausesPerTx)

		// create test db - will be automagically removed when the benchmark ends
		db, err := openTempMainDB(b.TempDir())
		require.NoError(b, err)

		// create blocks
		newChain, transactions := createPackedChain(b, db, blockCount, accounts, randomSignerFunc)

		// shuffle the transaction into a randomized order
		randomizedTransactions := shuffleSlice(transactions)
		b.Logf("About to process %d txs", len(randomizedTransactions))

		// run the benchmarks
		b.Run("getTransaction", func(b *testing.B) {
			benchmarkGetTransaction(b, newChain, randomizedTransactions)
		})

		b.Run("getReceipt", func(b *testing.B) {
			benchmarkGetReceipt(b, newChain, randomizedTransactions)
		})
	}
}

func BenchmarkFetchTx_RealDB_RandomSigners_OneClausePerTx(b *testing.B) {
	// create state accounts
	accounts := getCachedAccounts(b)

	// randomly pick a signer for signing the transactions
	randomSignerFunc := randomPickSignerFunc(accounts, createOneClausePerTx)

	// create test db - will be automagically removed when the benchmark ends
	db, err := openTempMainDB(b.TempDir())
	require.NoError(b, err)

	// create blocks
	newChain, transactions := createPackedChain(b, db, blockCount, accounts, randomSignerFunc)

	// shuffle the transaction into a randomized order
	randomizedTransactions := shuffleSlice(transactions)
	b.Logf("About to process %d txs", len(randomizedTransactions))

	// run the benchmarks
	b.Run("getTransaction", func(b *testing.B) {
		benchmarkGetTransaction(b, newChain, randomizedTransactions)
	})

	b.Run("getReceipt", func(b *testing.B) {
		benchmarkGetReceipt(b, newChain, randomizedTransactions)
	})
}

func BenchmarkFetchTx_RandomSigners_ManyClausesPerTx(b *testing.B) {
	for _, createManyClausesPerTx := range createManyTxFuncs { // create state accounts
		accounts := getCachedAccounts(b)
		// randomly pick a signer for signing the transactions
		randomSignerFunc := randomPickSignerFunc(accounts, createManyClausesPerTx)

		// create blocks
		newChain, transactions := createPackedChain(b, muxdb.NewMem(), blockCount, accounts, randomSignerFunc)

		// shuffle the transaction into a randomized order
		randomizedTransactions := shuffleSlice(transactions)
		b.Logf("About to process %d txs", len(randomizedTransactions))

		// run the benchmarks
		b.Run("getTransaction", func(b *testing.B) {
			benchmarkGetTransaction(b, newChain, randomizedTransactions)
		})

		b.Run("getReceipt", func(b *testing.B) {
			benchmarkGetReceipt(b, newChain, randomizedTransactions)
		})
	}
}

func BenchmarkFetchTx_RandomSigners_OneClausePerTx(b *testing.B) {
	// Setup phase: Not part of the benchmark timing
	b.StopTimer()

	// create state accounts
	accounts := getCachedAccounts(b)

	// randomly pick a signer for signing the transactions
	randomSignerFunc := randomPickSignerFunc(accounts, createOneClausePerTx)

	// create blocks
	newChain, transactions := createPackedChain(b, muxdb.NewMem(), blockCount, accounts, randomSignerFunc)

	// shuffle the transaction into a randomized order
	randomizedTransactions := shuffleSlice(transactions)
	b.Logf("About to process %d txs", len(randomizedTransactions))

	// run the benchmarks
	b.Run("getTransaction", func(b *testing.B) {
		benchmarkGetTransaction(b, newChain, randomizedTransactions)
	})

	b.Run("getReceipt", func(b *testing.B) {
		benchmarkGetReceipt(b, newChain, randomizedTransactions)
	})
}

func benchmarkGetTransaction(b *testing.B, thorChain *testchain.Chain, randTxs tx.Transactions) {
	mempool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{Limit: 10, LimitPerAccount: 16, MaxLifetime: 10 * time.Minute}, &thor.NoFork)
	transactionAPI := New(thorChain.Repo(), mempool)
	head := thorChain.Repo().BestBlockSummary().Header.ID()
	var err error

	// Measure memory usage
	b.ReportAllocs()

	// Benchmark execution
	b.ResetTimer()

	for _, randTx := range randTxs {
		_, err = transactionAPI.getRawTransaction(randTx.ID(), head, false)
		if err != nil {
			b.Fatalf("getRawTransaction failed: %v", err)
		}
	}
}

func benchmarkGetReceipt(b *testing.B, thorChain *testchain.Chain, randTxs tx.Transactions) {
	mempool := txpool.New(thorChain.Repo(), thorChain.Stater(), txpool.Options{Limit: 10, LimitPerAccount: 16, MaxLifetime: 10 * time.Minute}, &thor.NoFork)
	transactionAPI := New(thorChain.Repo(), mempool)
	head := thorChain.Repo().BestBlockSummary().Header.ID()
	var err error

	// Measure memory usage
	b.ReportAllocs()

	// Benchmark execution
	b.ResetTimer()

	for _, randTx := range randTxs {
		_, err = transactionAPI.getTransactionReceiptByID(randTx.ID(), head)
		if err != nil {
			b.Fatalf("getTransactionReceiptByID failed: %v", err)
		}
	}
}

func createPackedChain(b *testing.B, db *muxdb.MuxDB, noBlocks int, accounts []genesis.DevAccount, createTxFunc func(chain *testchain.Chain) (tx.Transactions, error)) (*testchain.Chain, tx.Transactions) {
	proposer := &accounts[0]

	// mock a fake chain for block production
	fakeChain, err := createChain(db, accounts)
	require.NoError(b, err)

	// pre-alloc blocks
	var transactions tx.Transactions

	// Start from the Genesis block
	previousBlock := fakeChain.GenesisBlock()
	for range noBlocks {
		newTxs, err := createTxFunc(fakeChain)
		require.NoError(b, err)
		previousBlock, err = packTxsIntoBlock(
			fakeChain,
			proposer,
			previousBlock,
			newTxs,
		)
		require.NoError(b, err)
		transactions = append(transactions, newTxs...)
	}

	return fakeChain, transactions
}

func createOneClausePerTx(signerPK *ecdsa.PrivateKey, thorChain *testchain.Chain) (tx.Transactions, error) {
	var transactions tx.Transactions
	gasUsed := uint64(0)
	for gasUsed < 9_500_000 {
		toAddr := datagen.RandAddress()
		cla := tx.NewClause(&toAddr).WithValue(big.NewInt(10000))
		b := tx.NewTxBuilder(tx.TypeLegacy)
		if gasUsed%2 == 0 {
			b = tx.NewTxBuilder(tx.TypeDynamicFee)
		}
		transaction := b.
			ChainTag(thorChain.Repo().ChainTag()).
			GasPriceCoef(1).
			MaxFeePerGas(big.NewInt(1000000)).
			MaxPriorityFeePerGas(big.NewInt(100)).
			Expiration(math.MaxUint32 - 1).
			Gas(21_000).
			Nonce(uint64(datagen.RandInt())).
			Clause(cla).
			BlockRef(tx.NewBlockRef(0)).
			MustBuild()

		sig, err := crypto.Sign(transaction.SigningHash().Bytes(), signerPK)
		if err != nil {
			return nil, err
		}
		transaction = transaction.WithSignature(sig)

		gasUsed += 21_000 // Gas per transaction
		transactions = append(transactions, transaction)
	}
	return transactions, nil
}

func createManyClausesPerTxLegacy(signerPK *ecdsa.PrivateKey, thorChain *testchain.Chain) (tx.Transactions, error) {
	var transactions tx.Transactions
	gasUsed := uint64(0)
	txGas := uint64(42_000)

	transactionBuilder := tx.NewTxBuilder(tx.TypeLegacy).
		ChainTag(thorChain.Repo().ChainTag()).
		GasPriceCoef(1).
		Expiration(math.MaxUint32 - 1).
		Nonce(uint64(datagen.RandInt())).
		BlockRef(tx.NewBlockRef(0))

	for ; gasUsed < 9_500_000; gasUsed += txGas {
		toAddr := datagen.RandAddress()
		transactionBuilder.Clause(tx.NewClause(&toAddr).WithValue(big.NewInt(10000)))
	}

	transaction := transactionBuilder.Gas(gasUsed).MustBuild()

	sig, err := crypto.Sign(transaction.SigningHash().Bytes(), signerPK)
	if err != nil {
		return nil, err
	}
	transaction = transaction.WithSignature(sig)

	transactions = append(transactions, transaction)

	return transactions, nil
}

func createManyClausesPerTxDynFee(signerPK *ecdsa.PrivateKey, thorChain *testchain.Chain) (tx.Transactions, error) {
	var transactions tx.Transactions
	gasUsed := uint64(0)
	txGas := uint64(42_000)

	transactionBuilder := tx.NewTxBuilder(tx.TypeDynamicFee).
		ChainTag(thorChain.Repo().ChainTag()).
		MaxFeePerGas(big.NewInt(1000000)).
		MaxPriorityFeePerGas(big.NewInt(100)).
		Expiration(math.MaxUint32 - 1).
		Nonce(uint64(datagen.RandInt())).
		BlockRef(tx.NewBlockRef(0))

	for ; gasUsed < 9_500_000; gasUsed += txGas {
		toAddr := datagen.RandAddress()
		transactionBuilder.Clause(tx.NewClause(&toAddr).WithValue(big.NewInt(10000)))
	}

	transaction := transactionBuilder.Gas(gasUsed).MustBuild()

	sig, err := crypto.Sign(transaction.SigningHash().Bytes(), signerPK)
	if err != nil {
		return nil, err
	}
	transaction = transaction.WithSignature(sig)

	transactions = append(transactions, transaction)

	return transactions, nil
}

func packTxsIntoBlock(thorChain *testchain.Chain, proposerAccount *genesis.DevAccount, parentBlk *block.Block, transactions tx.Transactions) (*block.Block, error) {
	p := packer.New(thorChain.Repo(), thorChain.Stater(), proposerAccount.Address, &proposerAccount.Address, thorChain.GetForkConfig())

	parentSum, err := thorChain.Repo().GetBlockSummary(parentBlk.Header().ID())
	if err != nil {
		return nil, err
	}

	flow, err := p.Schedule(parentSum, parentBlk.Header().Timestamp()+1)
	if err != nil {
		return nil, err
	}

	for _, transaction := range transactions {
		err = flow.Adopt(transaction)
		if err != nil {
			return nil, err
		}
	}

	b1, stage, receipts, err := flow.Pack(proposerAccount.PrivateKey, 0, false)
	if err != nil {
		return nil, err
	}

	if _, err := stage.Commit(); err != nil {
		return nil, err
	}

	if err := thorChain.Repo().AddBlock(b1, receipts, 0, true); err != nil {
		return nil, err
	}

	return b1, nil
}

func createChain(db *muxdb.MuxDB, accounts []genesis.DevAccount) (*testchain.Chain, error) {
	// Create the state manager (Stater) with the initialized database.
	stater := state.NewStater(db)

	authAccs := make([]genesis.Authority, 0, len(accounts))
	stateAccs := make([]genesis.Account, 0, len(accounts))

	for _, acc := range accounts {
		authAccs = append(authAccs, genesis.Authority{
			MasterAddress:   acc.Address,
			EndorsorAddress: acc.Address,
			Identity:        thor.BytesToBytes32([]byte("master")),
		})
		bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
		stateAccs = append(stateAccs, genesis.Account{
			Address: acc.Address,
			Balance: (*genesis.HexOrDecimal256)(bal),
			Energy:  (*genesis.HexOrDecimal256)(bal),
			Code:    "",
			Storage: nil,
		})
	}
	mbp := uint64(1_000)
	genConfig := genesis.CustomGenesis{
		LaunchTime: 1526400000,
		GasLimit:   thor.InitialGasLimit,
		ExtraData:  "",
		ForkConfig: &testchain.DefaultForkConfig,
		Authority:  authAccs,
		Accounts:   stateAccs,
		Params: genesis.Params{
			MaxBlockProposers: &mbp,
		},
	}

	builder, err := genesis.NewCustomNet(&genConfig)
	if err != nil {
		return nil, err
	}

	// Initialize the genesis and retrieve the genesis block
	//gene := genesis.NewDevnet()
	geneBlk, _, _, err := builder.Build(stater)
	if err != nil {
		return nil, err
	}

	// Create the repository which manages chain data, using the database and genesis block.
	repo, err := chain.NewRepository(db, geneBlk)
	if err != nil {
		return nil, err
	}

	// Create an inMemory logdb
	logDb, err := logdb.NewMem()
	if err != nil {
		return nil, err
	}

	return testchain.New(
		db,
		builder,
		solo.NewBFTEngine(repo),
		repo,
		stater,
		geneBlk,
		logDb,
		thor.NoFork,
	), nil
}

func randomPickSignerFunc(
	accounts []genesis.DevAccount,
	createTxFun func(signerPK *ecdsa.PrivateKey, thorChain *testchain.Chain) (tx.Transactions, error),
) func(chain *testchain.Chain) (tx.Transactions, error) {
	return func(chain *testchain.Chain) (tx.Transactions, error) {
		// Ensure there are accounts available
		if len(accounts) == 0 {
			return nil, fmt.Errorf("no accounts available to pick a random sender")
		}

		// Securely pick a random index
		maxLen := big.NewInt(int64(len(accounts)))
		randomIndex, err := rand.Int(rand.Reader, maxLen)
		if err != nil {
			return nil, fmt.Errorf("failed to generate random index: %v", err)
		}

		// Use the selected account to create transactions
		sender := accounts[randomIndex.Int64()]
		return createTxFun(sender.PrivateKey, chain)
	}
}

func createAccounts(b *testing.B, accountNo int) []genesis.DevAccount {
	var accs []genesis.DevAccount

	for range accountNo {
		pk, err := crypto.GenerateKey()
		require.NoError(b, err)
		addr := crypto.PubkeyToAddress(pk.PublicKey)
		accs = append(accs, genesis.DevAccount{Address: thor.Address(addr), PrivateKey: pk})
	}

	return accs
}

func openTempMainDB(dir string) (*muxdb.MuxDB, error) {
	cacheMB := normalizeCacheSize(4096)

	fdCache := suggestFDCache()

	opts := muxdb.Options{
		TrieNodeCacheSizeMB:        cacheMB,
		TrieCachedNodeTTL:          30, // 5min
		TrieDedupedPartitionFactor: math.MaxUint32,
		TrieWillCleanHistory:       true,
		OpenFilesCacheCapacity:     fdCache,
		ReadCacheMB:                256, // rely on os page cache other than huge db read cache.
		WriteBufferMB:              128,
	}

	// go-ethereum stuff
	// Ensure Go's GC ignores the database cache for trigger percentage
	totalCacheMB := cacheMB + opts.ReadCacheMB + opts.WriteBufferMB*2
	gogc := math.Max(10, math.Min(100, 50/(float64(totalCacheMB)/1024)))

	debug.SetGCPercent(int(gogc))

	if opts.TrieWillCleanHistory {
		opts.TrieHistPartitionFactor = 256
	} else {
		opts.TrieHistPartitionFactor = 524288
	}

	db, err := muxdb.Open(filepath.Join(dir, "maindb"), &opts)
	if err != nil {
		return nil, errors.Wrapf(err, "open main database [%v]", dir)
	}
	return db, nil
}

func normalizeCacheSize(sizeMB int) int {
	if sizeMB < 128 {
		sizeMB = 128
	}

	var mem gosigar.Mem
	if err := mem.Get(); err != nil {
		fmt.Println("failed to get total mem:", "err", err)
	} else {
		total := int(mem.Total / 1024 / 1024)
		half := total / 2

		// limit to not less than total/2 and up to total-2GB
		limitMB := max(total-2048, half)

		if sizeMB > limitMB {
			sizeMB = limitMB
			fmt.Println("cache size(MB) limited", "limit", limitMB)
		}
	}
	return sizeMB
}

func suggestFDCache() int {
	limit, err := fdlimit.Current()
	if err != nil {
		fmt.Println("unable to get fdlimit", "error", err)
		return 500
	}
	if limit <= 1024 {
		fmt.Println("low fd limit, increase it if possible", "limit", limit)
	}

	n := limit / 2
	if n > 5120 {
		return 5120
	}
	return n
}

func shuffleSlice(slice tx.Transactions) tx.Transactions {
	shuffled := make(tx.Transactions, len(slice))
	copy(shuffled, slice)

	for i := len(shuffled) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			panic(err) // Handle errors appropriately in real code
		}

		// Swap the current element with the random index
		j := int(n.Int64())
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled
}

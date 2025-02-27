// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer_test

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus/fork"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func createTx(txType tx.TxType, chainTag byte, gasPriceCoef uint8, expiration uint32, gas uint64, nonce uint64, dependsOn *thor.Bytes32, clause *tx.Clause, br tx.BlockRef) *tx.Transaction {
	builder := tx.NewTxBuilder(txType).
		ChainTag(chainTag).
		GasPriceCoef(gasPriceCoef).
		MaxFeePerGas(big.NewInt(thor.InitialBaseFee)).
		Expiration(expiration).
		Gas(gas).
		Nonce(nonce).
		DependsOn(dependsOn).
		Clause(clause).
		BlockRef(br)

	transaction := builder.MustBuild()

	signature, _ := crypto.Sign(transaction.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)

	return transaction.WithSignature(signature)
}

func TestAdopt(t *testing.T) {
	// Setup environment
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	g := genesis.NewDevnet()

	// Build genesis block
	b, _, _, _ := g.Build(stater)
	repo, _ := chain.NewRepository(db, b)

	// Common transaction setup
	chainTag := repo.ChainTag()
	addr := thor.BytesToAddress([]byte("to"))
	clause := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	// Create and adopt two transactions
	pkr := packer.New(repo, stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, thor.NoFork)
	sum, err := repo.GetBlockSummary(b.Header().ID())
	if err != nil {
		t.Fatal("Error getting block summary:", err)
	}

	flow, err := pkr.Schedule(sum, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal("Error scheduling:", err)
	}

	tx1 := createTx(tx.TypeLegacy, chainTag, 1, 10, 21000, 1, nil, clause, tx.NewBlockRef(0))
	if err := flow.Adopt(tx1); err != nil {
		t.Fatal("Error adopting tx1:", err)
	}

	tx2 := createTx(tx.TypeLegacy, chainTag, 1, 10, 21000, 2, (*thor.Bytes32)(tx1.ID().Bytes()), clause, tx.NewBlockRef(0))
	if err := flow.Adopt(tx2); err != nil {
		t.Fatal("Error adopting tx2:", err)
	}

	//Repeat transaction
	expectedErrorMessage := "known tx"
	if err := flow.Adopt(tx2); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}

	// Test dependency that does not exist
	tx3 := createTx(tx.TypeLegacy, chainTag, 1, 10, 21000, 2, (*thor.Bytes32)((thor.Bytes32{0x1}).Bytes()), clause, tx.NewBlockRef(0))
	expectedErrorMessage = "tx not adoptable now"
	if err := flow.Adopt(tx3); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}

	// Test Getters
	flow.ParentHeader()
	flow.When()
	flow.TotalScore()
}

func TestAdoptTypedTxs(t *testing.T) {
	// Setup environment
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	g := genesis.NewDevnet()

	// Build genesis block
	b, _, _, _ := g.Build(stater)
	repo, _ := chain.NewRepository(db, b)

	// Common transaction setup
	chainTag := repo.ChainTag()
	addr := thor.BytesToAddress([]byte("to"))
	clause := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	// Create and adopt two transactions
	pkr := packer.New(repo, stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, thor.ForkConfig{GALACTICA: 1})
	sum, err := repo.GetBlockSummary(b.Header().ID())
	if err != nil {
		t.Fatal("Error getting block summary:", err)
	}

	flow, err := pkr.Schedule(sum, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal("Error scheduling:", err)
	}

	tx1 := createTx(tx.TypeLegacy, chainTag, 1, 10, 21000, 1, nil, clause, tx.NewBlockRef(0))
	if err := flow.Adopt(tx1); err != nil {
		t.Fatal("Error adopting tx1:", err)
	}

	tx2 := createTx(tx.TypeDynamicFee, chainTag, 1, 10, 21000, 2, (*thor.Bytes32)(tx1.ID().Bytes()), clause, tx.NewBlockRef(0))
	if err := flow.Adopt(tx2); err != nil {
		t.Fatal("Error adopting tx2:", err)
	}

	//Repeat transaction
	expectedErrorMessage := "known tx"
	if err := flow.Adopt(tx2); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}

	// Test dependency that does not exist
	tx3 := createTx(tx.TypeDynamicFee, chainTag, 1, 10, 21000, 2, (*thor.Bytes32)((thor.Bytes32{0x1}).Bytes()), clause, tx.NewBlockRef(0))
	expectedErrorMessage = "tx not adoptable now"
	if err := flow.Adopt(tx3); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}
}

func TestPack(t *testing.T) {
	db := muxdb.NewMem()
	g := genesis.NewDevnet()

	stater := state.NewStater(db)
	parent, _, _, _ := g.Build(stater)

	repo, _ := chain.NewRepository(db, parent)

	forkConfig := thor.NoFork
	forkConfig.BLOCKLIST = 0
	forkConfig.VIP214 = 0
	forkConfig.FINALITY = 0

	proposer := genesis.DevAccounts()[0]
	p := packer.New(repo, stater, proposer.Address, &proposer.Address, forkConfig)
	parentSum, _ := repo.GetBlockSummary(parent.Header().ID())
	flow, _ := p.Schedule(parentSum, parent.Header().Timestamp()+100*thor.BlockInterval)

	flow.Pack(proposer.PrivateKey, 0, false)

	//Test with shouldVote
	flow.Pack(proposer.PrivateKey, 0, true)

	//Test wrong private key
	expectedErrorMessage := "private key mismatch"
	if _, _, _, err := flow.Pack(genesis.DevAccounts()[1].PrivateKey, 0, false); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}
}

func TestPackAfterGalacticaFork(t *testing.T) {
	db := muxdb.NewMem()
	g := genesis.NewDevnet()

	stater := state.NewStater(db)
	parent, _, _, _ := g.Build(stater)

	repo, _ := chain.NewRepository(db, parent)

	forkConfig := thor.NoFork
	forkConfig.BLOCKLIST = 0
	forkConfig.VIP214 = 0
	forkConfig.FINALITY = 0
	forkConfig.GALACTICA = 2

	proposer := genesis.DevAccounts()[0]
	p := packer.New(repo, stater, proposer.Address, &proposer.Address, forkConfig)
	parentSum, _ := repo.GetBlockSummary(parent.Header().ID())
	flow, _ := p.Schedule(parentSum, parent.Header().Timestamp()+100*thor.BlockInterval)

	// Block 1: Galactica is not enabled
	block, stg, receipts, err := flow.Pack(proposer.PrivateKey, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, uint32(1), block.Header().Number())
	assert.Nil(t, block.Header().BaseFee())

	if _, err := stg.Commit(); err != nil {
		t.Fatal("Error committing state:", err)
	}
	if err := repo.AddBlock(block, receipts, 0, true); err != nil {
		t.Fatal("Error adding block:", err)
	}

	// Block 2: Galactica is enabled
	parentSum, _ = repo.GetBlockSummary(block.Header().ID())
	flow, _ = p.Schedule(parentSum, block.Header().Timestamp()+100*thor.BlockInterval)
	block, _, _, err = flow.Pack(proposer.PrivateKey, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, uint32(2), block.Header().Number())
	assert.Equal(t, big.NewInt(thor.InitialBaseFee), block.Header().BaseFee())

	// Adopt a tx which has not enough max fee to cover for base fee
	badTx := tx.NewTxBuilder(tx.TypeDynamicFee).ChainTag(repo.ChainTag()).Gas(21000).MaxFeePerGas(big.NewInt(thor.InitialBaseFee - 1)).MaxPriorityFeePerGas(common.Big1).Expiration(100).MustBuild()
	badTx = tx.MustSign(badTx, genesis.DevAccounts()[0].PrivateKey)
	expectedErrorMessage := "tx not adoptable now"
	if err := flow.Adopt(badTx); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}
}

func TestAdoptErr(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	launchTime := uint64(1526400000)
	g := new(genesis.Builder).
		GasLimit(0).
		Timestamp(launchTime).
		State(func(state *state.State) error {
			bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			builtin.Params.Native(state).Set(thor.KeyExecutorAddress, new(big.Int).SetBytes(genesis.DevAccounts()[0].Address[:]))
			for _, acc := range genesis.DevAccounts() {
				state.SetBalance(acc.Address, bal)
				state.SetEnergy(acc.Address, bal, launchTime)
				builtin.Authority.Native(state).Add(acc.Address, acc.Address, thor.Bytes32{})
			}
			return nil
		})

	// Build genesis block
	b, _, _, _ := g.Build(stater)

	repo, _ := chain.NewRepository(db, b)

	// Common transaction setup
	addr := thor.BytesToAddress([]byte("to"))
	clause := tx.NewClause(&addr).WithValue(big.NewInt(10000))

	pkr := packer.New(repo, stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, thor.NoFork)
	sum, _ := repo.GetBlockSummary(b.Header().ID())

	flow, _ := pkr.Schedule(sum, uint64(time.Now().Unix()))

	// Test chain tag mismatch
	tx1 := createTx(tx.TypeLegacy, byte(0xFF), 1, 10, 21000, 1, nil, clause, tx.NewBlockRef(0))
	expectedErrorMessage := "bad tx: chain tag mismatch"
	if err := flow.Adopt(tx1); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}

	// Test wrong block reference
	tx2 := createTx(tx.TypeLegacy, repo.ChainTag(), 1, 10, 21000, 1, nil, clause, tx.NewBlockRef(1000))
	expectedErrorMessage = "tx not adoptable now"
	if err := flow.Adopt(tx2); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}

	// Test exceeded gas limit
	tx3 := createTx(tx.TypeLegacy, repo.ChainTag(), 1, 0, 1, 1, nil, clause, tx.NewBlockRef(1))
	expectedErrorMessage = "gas limit reached"
	if err := flow.Adopt(tx3); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}
}

func TestAdoptErrorAfterGalactica(t *testing.T) {
	forks := thor.ForkConfig{GALACTICA: 2}
	chain, err := testchain.NewWithFork(forks)
	assert.NoError(t, err)

	// Try to adopt a dyn fee tx before galactica fork activates - FAILS
	tr := tx.NewTxBuilder(tx.TypeDynamicFee).ChainTag(chain.Repo().ChainTag()).Gas(21000).Expiration(100).MustBuild()
	tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
	err = chain.MintBlock(genesis.DevAccounts()[0], tr)

	expectedErrMsg := "unable to adopt tx into block: bad tx: invalid tx type"
	assert.Equal(t, expectedErrMsg, err.Error())

	// Try to adopt a legacy tx - SUCCESS
	tr = tx.NewTxBuilder(tx.TypeLegacy).ChainTag(chain.Repo().ChainTag()).Gas(21000).Expiration(100).MustBuild()
	tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
	err = chain.MintBlock(genesis.DevAccounts()[0], tr)
	assert.NoError(t, err)

	// Try to adopt a dyn fee tx after galactica fork activates - SUCCESS
	tr = tx.NewTxBuilder(tx.TypeDynamicFee).ChainTag(chain.Repo().ChainTag()).MaxFeePerGas(big.NewInt(thor.InitialBaseFee)).Gas(21000).Expiration(100).MustBuild()
	tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
	err = chain.MintBlock(genesis.DevAccounts()[0], tr)
	assert.NoError(t, err)

	// Try to adopt a dyn fee tx with max fee per gas less than base fee - FAILS
	best, err := chain.BestBlock()
	assert.NoError(t, err)
	expectedBaseFee := fork.CalcBaseFee(&forks, best.Header())
	notEnoughBaseFee := new(big.Int).Sub(expectedBaseFee, common.Big1)

	tr = tx.NewTxBuilder(tx.TypeDynamicFee).ChainTag(chain.Repo().ChainTag()).Nonce(2).MaxFeePerGas(notEnoughBaseFee).Gas(21000).Expiration(100).MustBuild()
	tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
	err = chain.MintBlock(genesis.DevAccounts()[0], tr)
	expectedErrMsg = "unable to adopt tx into block: tx not adoptable now"
	assert.Equal(t, expectedErrMsg, err.Error())

	// Try to adopt a dyn fee with just the right amount of max fee per gas - SUCCESS
	tr = tx.NewTxBuilder(tx.TypeDynamicFee).ChainTag(chain.Repo().ChainTag()).Nonce(2).MaxFeePerGas(expectedBaseFee).Gas(21000).Expiration(100).MustBuild()
	tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
	err = chain.MintBlock(genesis.DevAccounts()[0], tr)
	assert.NoError(t, err)

	// Try to adopt a dyn fee with max fee = base fee + maxPriorityFee
	best, err = chain.BestBlock()
	assert.NoError(t, err)
	expectedBaseFee = fork.CalcBaseFee(&forks, best.Header())
	maxPriorityFee := big.NewInt(10_000)
	maxFee := new(big.Int).Add(expectedBaseFee, maxPriorityFee)
	tr = tx.NewTxBuilder(tx.TypeDynamicFee).ChainTag(chain.Repo().ChainTag()).Nonce(3).MaxFeePerGas(maxFee).MaxPriorityFeePerGas(maxPriorityFee).Gas(21000).Expiration(100).MustBuild()
	tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
	err = chain.MintBlock(genesis.DevAccounts()[0], tr)
	assert.NoError(t, err)
}

func TestAdoptAfterGalacticaLowerBaseFeeThreshold(t *testing.T) {
	chain, err := testchain.NewWithFork(thor.ForkConfig{GALACTICA: 1})
	assert.NoError(t, err)

	tr := tx.NewTxBuilder(tx.TypeLegacy).ChainTag(chain.Repo().ChainTag()).Gas(21000).Expiration(100).MustBuild()
	tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
	err = chain.MintBlock(genesis.DevAccounts()[0], tr)
	assert.NoError(t, err)

	for i := 0; i < 10000; i++ {
		best, err := chain.BestBlock()
		assert.NoError(t, err)
		expectedBaseFee := fork.CalcBaseFee(&thor.ForkConfig{}, best.Header())
		tr = tx.NewTxBuilder(tx.TypeDynamicFee).ChainTag(chain.Repo().ChainTag()).Nonce(uint64(i + 2)).MaxFeePerGas(expectedBaseFee).Gas(21000).Expiration(1000000).MustBuild()
		tr = tx.MustSign(tr, genesis.DevAccounts()[0].PrivateKey)
		err = chain.MintBlock(genesis.DevAccounts()[0], tr)
		assert.NoError(t, err)
	}
	best, _ := chain.BestBlock()
	fmt.Println(best.Header().BaseFee())
}

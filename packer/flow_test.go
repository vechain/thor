// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func createTx(chainTag byte, gasPriceCoef uint8, expiration uint32, gas uint64, nonce uint64, dependsOn *thor.Bytes32, clause *tx.Clause, br tx.BlockRef) *tx.Transaction {
	builder := new(tx.Builder).
		ChainTag(chainTag).
		GasPriceCoef(gasPriceCoef).
		Expiration(expiration).
		Gas(gas).
		Nonce(nonce).
		DependsOn(dependsOn).
		Clause(clause).
		BlockRef(br)

	transaction := builder.Build()

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
	pkr := packer.New(repo, stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, &thor.NoFork)
	sum, err := repo.GetBlockSummary(b.Header().ID())
	if err != nil {
		t.Fatal("Error getting block summary:", err)
	}

	flow, err := pkr.Schedule(sum, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal("Error scheduling:", err)
	}

	tx1 := createTx(chainTag, 1, 10, 21000, 1, nil, clause, tx.NewBlockRef(0))
	if err := flow.Adopt(tx1); err != nil {
		t.Fatal("Error adopting tx1:", err)
	}

	tx2 := createTx(chainTag, 1, 10, 21000, 2, (*thor.Bytes32)(tx1.ID().Bytes()), clause, tx.NewBlockRef(0))
	if err := flow.Adopt(tx2); err != nil {
		t.Fatal("Error adopting tx2:", err)
	}

	//Repeat transaction
	expectedErrorMessage := "known tx"
	if err := flow.Adopt(tx2); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}

	// Test dependency that does not exist
	tx3 := createTx(chainTag, 1, 10, 21000, 2, (*thor.Bytes32)((thor.Bytes32{0x1}).Bytes()), clause, tx.NewBlockRef(0))
	expectedErrorMessage = "tx not adoptable now"
	if err := flow.Adopt(tx3); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}

	// Test Getters
	flow.ParentHeader()
	flow.When()
	flow.TotalScore()
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
	p := packer.New(repo, stater, proposer.Address, &proposer.Address, &forkConfig)
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

func TestAdoptErr(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	launchTime := uint64(1526400000)
	g := new(genesis.Builder).
		GasLimit(0).
		Timestamp(launchTime).
		ForkConfig(&thor.NoFork).
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

	pkr := packer.New(repo, stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, &thor.SoloFork)
	sum, _ := repo.GetBlockSummary(b.Header().ID())

	flow, _ := pkr.Schedule(sum, uint64(time.Now().Unix()))

	// Test chain tag mismatch
	tx1 := createTx(byte(0xFF), 1, 10, 21000, 1, nil, clause, tx.NewBlockRef(0))
	expectedErrorMessage := "bad tx: chain tag mismatch"
	if err := flow.Adopt(tx1); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}

	// Test wrong block reference
	tx2 := createTx(repo.ChainTag(), 1, 10, 1, 21000, nil, clause, tx.NewBlockRef(1000))
	expectedErrorMessage = "tx not adoptable now"
	if err := flow.Adopt(tx2); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}

	// Test exceeded gas limit
	tx3 := createTx(repo.ChainTag(), 1, 0, 1, 1, nil, clause, tx.NewBlockRef(1))
	expectedErrorMessage = "gas limit reached"
	if err := flow.Adopt(tx3); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}

	thor.MockBlocklist([]string{genesis.DevAccounts()[9].Address.String()})
	// Test origin blacklisted
	builder := new(tx.Builder).
		ChainTag(repo.ChainTag()).
		GasPriceCoef(1).
		Expiration(0).
		Gas(10e18).
		Nonce(nonce).
		Clause(clause).
		BlockRef(tx.NewBlockRef(1))
	tx4 := tx.MustSign(builder.Build(), genesis.DevAccounts()[9].PrivateKey)

	expectedErrorMessage = "bad tx: tx origin blocked"
	if err := flow.Adopt(tx4); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}

	// Test delegator blacklisted
	builder = new(tx.Builder).
		ChainTag(repo.ChainTag()).
		GasPriceCoef(1).
		Expiration(0).
		Gas(10e18).
		Nonce(nonce).
		Clause(clause).
		Features(tx.Features(0x01)).
		BlockRef(tx.NewBlockRef(1))
	tx5 := tx.MustSignDelegated(builder.Build(), genesis.DevAccounts()[8].PrivateKey, genesis.DevAccounts()[9].PrivateKey)

	expectedErrorMessage = "bad tx: tx delegator blocked"
	if err := flow.Adopt(tx5); err.Error() != expectedErrorMessage {
		t.Fatalf("Expected error message: '%s', but got: '%s'", expectedErrorMessage, err.Error())
	}
}

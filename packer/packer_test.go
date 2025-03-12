// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer_test

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

func M(args ...interface{}) []interface{} {
	return args
}

type txIterator struct {
	chainTag byte
	i        int
}

var nonce = uint64(time.Now().UnixNano())

func (ti *txIterator) HasNext() bool {
	return ti.i < 100
}
func (ti *txIterator) Next() *tx.Transaction {
	ti.i++

	accs := genesis.DevAccounts()
	a0 := accs[0]
	a1 := accs[1]

	method, _ := builtin.Energy.ABI.MethodByName("transfer")

	data, _ := method.EncodeInput(a1.Address, big.NewInt(1))

	trx := tx.NewTxBuilder(tx.TypeLegacy).
		ChainTag(ti.chainTag).
		Clause(tx.NewClause(&builtin.Energy.Address).WithData(data)).
		Gas(300000).GasPriceCoef(0).Nonce(nonce).Expiration(math.MaxUint32).MustBuild()
	trx = tx.MustSign(trx, a0.PrivateKey)
	nonce++

	return trx
}

func (ti *txIterator) OnProcessed(_ thor.Bytes32, _ error) {
}

func TestP(t *testing.T) {
	db := muxdb.NewMem()

	g := genesis.NewDevnet()
	b0, _, _, _ := g.Build(state.NewStater(db))

	repo, _ := chain.NewRepository(db, b0)

	a1 := genesis.DevAccounts()[0]

	start := time.Now().UnixNano()
	stater := state.NewStater(db)
	// f, err := os.Create("/tmp/ppp")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()

	for {
		best := repo.BestBlockSummary()
		p := packer.New(repo, stater, a1.Address, &a1.Address, thor.NoFork, 0)
		flow, err := p.Schedule(best, uint64(time.Now().Unix()))
		if err != nil {
			t.Fatal(err)
		}
		iter := &txIterator{chainTag: b0.Header().ID()[31]}
		for iter.HasNext() {
			tx := iter.Next()
			flow.Adopt(tx)
		}

		blk, stage, receipts, _ := flow.Pack(genesis.DevAccounts()[0].PrivateKey, 0, false)
		root, _ := stage.Commit()
		assert.Equal(t, root, blk.Header().StateRoot())
		_, _, err = consensus.New(repo, stater, thor.NoFork).Process(best, blk, uint64(time.Now().Unix()*2), 0)
		assert.Nil(t, err)

		if err := repo.AddBlock(blk, receipts, 0, true); err != nil {
			t.Fatal(err)
		}

		if time.Now().UnixNano() > start+1000*1000*1000*1 {
			break
		}
	}

	best := repo.BestBlockSummary()
	assert.NotNil(t, best)
	assert.True(t, best.Header.Number() > 0)
	assert.True(t, best.Header.GasUsed() > 0)
}

func TestForkVIP191(t *testing.T) {
	db := muxdb.NewMem()

	launchTime := uint64(time.Now().Unix())
	a1 := genesis.DevAccounts()[0]
	stater := state.NewStater(db)

	b0, _, _, err := new(genesis.Builder).
		GasLimit(thor.InitialGasLimit).
		Timestamp(launchTime).
		ForkConfig(thor.NoFork).
		State(func(state *state.State) error {
			// setup builtin contracts
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			state.SetCode(builtin.Extension.Address, builtin.Extension.RuntimeBytecodes())

			bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
			state.SetBalance(a1.Address, bal)
			state.SetEnergy(a1.Address, bal, launchTime)

			builtin.Authority.Native(state).Add(a1.Address, a1.Address, thor.BytesToBytes32([]byte{}))
			return nil
		}).
		Build(stater)

	if err != nil {
		t.Fatal(err)
	}

	repo, _ := chain.NewRepository(db, b0)

	fc := thor.NoFork
	fc.VIP191 = 1

	best := repo.BestBlockSummary()
	p := packer.New(repo, stater, a1.Address, &a1.Address, fc, 0)
	flow, err := p.Schedule(best, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}

	blk, stage, receipts, _ := flow.Pack(a1.PrivateKey, 0, false)
	root, _ := stage.Commit()
	assert.Equal(t, root, blk.Header().StateRoot())

	_, _, err = consensus.New(repo, stater, fc).Process(best, blk, uint64(time.Now().Unix()*2), 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.AddBlock(blk, receipts, 0, false); err != nil {
		t.Fatal(err)
	}

	headState := state.New(db, trie.Root{Hash: blk.Header().StateRoot(), Ver: trie.Version{Major: blk.Header().Number()}})

	assert.Equal(t, M(builtin.Extension.V2.RuntimeBytecodes(), nil), M(headState.GetCode(builtin.Extension.Address)))

	geneState := state.New(db, trie.Root{Hash: b0.Header().StateRoot()})

	assert.Equal(t, M(builtin.Extension.RuntimeBytecodes(), nil), M(geneState.GetCode(builtin.Extension.Address)))
}

func TestBlocklist(t *testing.T) {
	db := muxdb.NewMem()

	g := genesis.NewDevnet()
	b0, _, _, _ := g.Build(state.NewStater(db))

	repo, _ := chain.NewRepository(db, b0)

	a0 := genesis.DevAccounts()[0]
	a1 := genesis.DevAccounts()[1]

	stater := state.NewStater(db)

	forkConfig := thor.ForkConfig{
		VIP191:    math.MaxUint32,
		ETH_CONST: math.MaxUint32,
		BLOCKLIST: 0,
		GALACTICA: math.MaxUint32,
	}

	thor.MockBlocklist([]string{a0.Address.String()})

	best := repo.BestBlockSummary()
	p := packer.New(repo, stater, a0.Address, &a0.Address, forkConfig, 0)
	flow, err := p.Schedule(best, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}

	tx0 := tx.NewTxBuilder(tx.TypeLegacy).
		ChainTag(repo.ChainTag()).
		Clause(tx.NewClause(&a1.Address)).
		Gas(300000).GasPriceCoef(0).Nonce(0).Expiration(math.MaxUint32).MustBuild()
	sig0, _ := crypto.Sign(tx0.SigningHash().Bytes(), a0.PrivateKey)
	tx0 = tx0.WithSignature(sig0)

	err = flow.Adopt(tx0)
	assert.True(t, packer.IsBadTx(err))
	assert.Equal(t, err.Error(), "bad tx: tx origin blocked")

	sig1, _ := crypto.Sign(tx0.SigningHash().Bytes(), a1.PrivateKey)
	tx1 := tx0.WithSignature(sig1)

	err = flow.Adopt(tx1)
	if err != nil {
		t.Fatal("adopt tx from non-blocked origin should not return error")
	}
}

func TestMock(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	g := genesis.NewDevnet()

	b0, _, _, _ := g.Build(stater)
	repo, _ := chain.NewRepository(db, b0)

	a0 := genesis.DevAccounts()[0]

	p := packer.New(repo, stater, a0.Address, &a0.Address, thor.NoFork, 0)

	best := repo.BestBlockSummary()

	// Create a packing flow mock with header gas limit
	_, err := p.Mock(best, uint64(time.Now().Unix()), b0.Header().GasLimit())
	if err != nil {
		t.Fatal("Failure to create a packing flow mock")
	}

	// Create a packing flow mock with 0 gas limit
	_, err = p.Mock(best, uint64(time.Now().Unix()), 0)
	if err != nil {
		t.Fatal("Failure to create a packing flow mock")
	}
}

func TestSetGasLimit(t *testing.T) {
	db := muxdb.NewMem()

	g := genesis.NewDevnet()
	stater := state.NewStater(db)
	b0, _, _, _ := g.Build(stater)
	repo, _ := chain.NewRepository(db, b0)

	a0 := genesis.DevAccounts()[0]

	p := packer.New(repo, stater, a0.Address, &a0.Address, thor.NoFork, 0)

	// This is just for code coverage purposes. There is no getter function for targetGasLimit to test the function.
	p.SetTargetGasLimit(0xFFFF)
}

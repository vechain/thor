// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package packer_test

import (
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/consensus"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func M(args ...interface{}) []interface{} {
	return args
}

type txIterator struct {
	chainTag byte
	i        int
}

var nonce uint64 = uint64(time.Now().UnixNano())

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

	tx := new(tx.Builder).
		ChainTag(ti.chainTag).
		Clause(tx.NewClause(&builtin.Energy.Address).WithData(data)).
		Gas(300000).GasPriceCoef(0).Nonce(nonce).Expiration(math.MaxUint32).Build()
	nonce++
	sig, _ := crypto.Sign(tx.SigningHash().Bytes(), a0.PrivateKey)
	tx = tx.WithSignature(sig)

	return tx
}

func (ti *txIterator) OnProcessed(txID thor.Bytes32, err error) {
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
		p := packer.New(repo, stater, a1.Address, &a1.Address, thor.NoFork)
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

		if err := repo.AddBlock(blk, receipts, 0); err != nil {
			t.Fatal(err)
		}
		repo.SetBestBlockID(blk.Header().ID())

		if time.Now().UnixNano() > start+1000*1000*1000*1 {
			break
		}
	}

	best := repo.BestBlockSummary()
	fmt.Println(best.Header.Number(), best.Header.GasUsed())
	//	fmt.Println(best)
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
	p := packer.New(repo, stater, a1.Address, &a1.Address, fc)
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

	if err := repo.AddBlock(blk, receipts, 0); err != nil {
		t.Fatal(err)
	}

	headState := state.New(db, blk.Header().StateRoot(), blk.Header().Number(), 0, 0)

	assert.Equal(t, M(builtin.Extension.V2.RuntimeBytecodes(), nil), M(headState.GetCode(builtin.Extension.Address)))

	geneState := state.New(db, b0.Header().StateRoot(), 0, 0, 0)

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
	}

	thor.MockBlocklist([]string{a0.Address.String()})

	best := repo.BestBlockSummary()
	p := packer.New(repo, stater, a0.Address, &a0.Address, forkConfig)
	flow, err := p.Schedule(best, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}

	tx0 := new(tx.Builder).
		ChainTag(repo.ChainTag()).
		Clause(tx.NewClause(&a1.Address)).
		Gas(300000).GasPriceCoef(0).Nonce(0).Expiration(math.MaxUint32).Build()
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

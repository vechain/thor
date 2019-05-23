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
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

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

	kv, _ := lvldb.NewMem()
	defer kv.Close()

	g := genesis.NewDevnet()
	b0, _, _ := g.Build(state.NewCreator(kv))

	c, _ := chain.New(kv, b0)

	a1 := genesis.DevAccounts()[0]

	start := time.Now().UnixNano()
	stateCreator := state.NewCreator(kv)
	// f, err := os.Create("/tmp/ppp")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()

	for {
		best := c.BestBlock()
		p := packer.New(c, stateCreator, a1.Address, &a1.Address, thor.NoFork)
		flow, err := p.Schedule(best.Header(), uint64(time.Now().Unix()))
		if err != nil {
			t.Fatal(err)
		}
		iter := &txIterator{chainTag: b0.Header().ID()[31]}
		for iter.HasNext() {
			tx := iter.Next()
			flow.Adopt(tx)
		}

		blk, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey)
		root, _ := stage.Commit()
		assert.Equal(t, root, blk.Header().StateRoot())
		fmt.Println(consensus.New(c, stateCreator, thor.NoFork).Process(blk, uint64(time.Now().Unix()*2)))

		if _, err := c.AddBlock(blk, receipts); err != nil {
			t.Fatal(err)
		}

		if time.Now().UnixNano() > start+1000*1000*1000*1 {
			break
		}
	}

	best := c.BestBlock()
	fmt.Println(best.Header().Number(), best.Header().GasUsed())
	//	fmt.Println(best)
}

func TestForkVIP191(t *testing.T) {
	kv, _ := lvldb.NewMem()
	defer kv.Close()

	launchTime := uint64(time.Now().Unix())
	a1 := genesis.DevAccounts()[0]
	stateCreator := state.NewCreator(kv)

	b0, _, err := new(genesis.Builder).
		GasLimit(thor.InitialGasLimit).
		Timestamp(launchTime).
		State(func(state *state.State) error {
			// setup builtin contracts
			state.SetCode(builtin.Authority.Address, builtin.Authority.RuntimeBytecodes())
			state.SetCode(builtin.Extension.Address, builtin.Extension.RuntimeBytecodes())

			bal, _ := new(big.Int).SetString("1000000000000000000000000000", 10)
			state.SetBalance(a1.Address, bal)
			state.SetEnergy(a1.Address, bal, launchTime)

			_ = builtin.Authority.Native(state).Add(a1.Address, a1.Address, thor.BytesToBytes32([]byte{}))
			return nil
		}).
		Build(stateCreator)

	if err != nil {
		t.Fatal(err)
	}

	c, _ := chain.New(kv, b0)

	best := c.BestBlock()
	p := packer.New(c, stateCreator, a1.Address, &a1.Address, thor.ForkConfig{
		FixTransferLog: 0,
		VIP191:         1,
	})
	flow, err := p.Schedule(best.Header(), uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}

	blk, stage, receipts, err := flow.Pack(a1.PrivateKey)
	root, _ := stage.Commit()
	assert.Equal(t, root, blk.Header().StateRoot())

	_, _, err = consensus.New(c, stateCreator, thor.ForkConfig{}).Process(blk, uint64(time.Now().Unix()*2))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.AddBlock(blk, receipts); err != nil {
		t.Fatal(err)
	}

	headState, err := state.NewCreator(kv).NewState(blk.Header().StateRoot())
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, headState.GetCode(builtin.Extension.Address), builtin.ExtensionV2.RuntimeBytecodes())

	geneState, err := state.NewCreator(kv).NewState(b0.Header().StateRoot())

	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, geneState.GetCode(builtin.Extension.Address), builtin.Extension.RuntimeBytecodes())
}

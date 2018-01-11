package genesis_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	State "github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestGenesis(t *testing.T) {
	assert := assert.New(t)
	kv, _ := lvldb.NewMem()
	defer kv.Close()
	state, _ := State.New(thor.Hash{}, kv)
	block, _ := genesis.Build(state)

	state, _ = State.New(block.Header().StateRoot(), kv)
	assert.True(len(state.GetCode(contracts.Authority.Address)) > 0)
}

func BenchmarkChargeEnergy(b *testing.B) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)
	blk, err := genesis.Build(st)
	if err != nil {
		b.Fatal(err)
	}

	root := blk.Header().StateRoot()

	for n := 0; n < b.N; n++ {
		st, err := state.New(root, kv)
		if err != nil {
			b.Fatal(err)
		}
		rt := runtime.New(st, &block.Header{}, func(uint64) thor.Hash { return thor.Hash{} })
		data, err := contracts.Energy.ABI.Pack(
			"charge",
			thor.BytesToAddress([]byte("acc1")),
			big.NewInt(1),
		)
		if err != nil {
			b.Fatal(err)
		}

		gas := uint64(1000000)
		// cost about  49165 gas
		out := rt.Execute(&tx.Clause{
			To:   &contracts.Energy.Address,
			Data: data,
		}, 0, gas, thor.GodAddress, new(big.Int), thor.Hash{})
		if out.VMErr != nil {
			b.Fatal(out.VMErr)
		}

		root, err = st.Stage().Commit()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConsumeEnergy(b *testing.B) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	_, err := genesis.Build(st)
	if err != nil {
		b.Fatal(err)
	}

	rt := runtime.New(st, &block.Header{}, func(uint64) thor.Hash { return thor.Hash{} })
	data, err := contracts.Energy.ABI.Pack(
		"charge",
		thor.BytesToAddress([]byte("acc1")),
		big.NewInt(1000*1000*1000*1000),
	)
	if err != nil {
		b.Fatal(err)
	}
	out := rt.Execute(&tx.Clause{
		To:   &contracts.Energy.Address,
		Data: data,
	}, 0, 1000000, thor.GodAddress, new(big.Int), thor.Hash{})
	if out.VMErr != nil {
		b.Fatal(out.VMErr)
	}

	root, err := st.Stage().Commit()
	if err != nil {
		b.Fatal(out.VMErr)
	}

	for n := 0; n < b.N; n++ {
		st, err := state.New(root, kv)
		if err != nil {
			panic(err)
		}
		rt := runtime.New(st, &block.Header{}, func(uint64) thor.Hash { return thor.Hash{} })
		data, err := contracts.Energy.ABI.Pack(
			"consume",
			thor.BytesToAddress([]byte("acc1")),
			thor.Address{},
			big.NewInt(1),
		)
		if err != nil {
			panic(err)
		}

		gas := uint64(1000000)
		// cost about  49165 gas
		out := rt.Execute(&tx.Clause{
			To:   &contracts.Energy.Address,
			Data: data,
		}, 0, gas, thor.GodAddress, new(big.Int), thor.Hash{})
		if out.VMErr != nil {
			panic(out.VMErr)
		}

		root, err = st.Stage().Commit()
		if err != nil {
			panic(err)
		}
	}
}

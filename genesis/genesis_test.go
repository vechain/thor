package genesis_test

import (
	"fmt"
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func TestGenesis(t *testing.T) {
	assert := assert.New(t)
	kv, _ := lvldb.NewMem()
	defer kv.Close()
	st, _ := state.New(thor.Hash{}, kv)
	block, err := genesis.Build(st)
	if err != nil {
		t.Fatal(err)
	}

	st, _ = state.New(block.Header().StateRoot(), kv)
	assert.True(len(st.GetCode(contracts.Authority.Address)) > 0)
}

func BenchmarkChargeEnergy(b *testing.B) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)
	_, err := genesis.Build(st)
	if err != nil {
		b.Fatal(err)
	}

	for n := 0; n < b.N; n++ {
		// st, err := state.New(root, kv)
		// if err != nil {
		// 	b.Fatal(err)
		// }
		rt := runtime.New(st,
			thor.Address{}, 0, 0, 0, func(uint32) thor.Hash { return thor.Hash{} })

		gas := uint64(1000000)
		// cost about  49165 gas
		out := rt.Call(
			contracts.Energy.PackCharge(
				thor.BytesToAddress([]byte("acc1")),
				big.NewInt(1),
			),
			0, gas, contracts.Energy.Address, new(big.Int), thor.Hash{})
		if out.VMErr != nil {
			b.Fatal(out.VMErr)
		}

		// root, err = st.Stage().Commit()
		// if err != nil {
		// 	b.Fatal(err)
		// }
	}
}

func BenchmarkConsumeEnergy(b *testing.B) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	_, err := genesis.Build(st)
	if err != nil {
		b.Fatal(err)
	}

	rt := runtime.New(st,
		thor.Address{}, 0, 0, 0, func(uint32) thor.Hash { return thor.Hash{} })

	out := rt.Call(
		contracts.Energy.PackCharge(
			thor.BytesToAddress([]byte("acc1")),
			big.NewInt(1000*1000*1000*1000),
		),
		0, 1000000, contracts.Energy.Address, new(big.Int), thor.Hash{})
	if out.VMErr != nil {
		b.Fatal(out.VMErr)
	}

	out = rt.StaticCall(contracts.Energy.PackBalanceOf(thor.BytesToAddress([]byte("acc1"))),
		0, math.MaxUint64, thor.Address{}, &big.Int{}, thor.Hash{})
	var bal *big.Int
	fmt.Println(contracts.Energy.ABI.Unpack(&bal, "balanceOf", out.Value))
	fmt.Println(bal)

	// root, err := st.Stage().Commit()
	// if err != nil {
	// 	b.Fatal(out.VMErr)
	// }

	for n := 0; n < b.N; n++ {
		// st, err := state.New(root, kv)
		// if err != nil {
		// 	panic(err)
		// }
		rt := runtime.New(st,
			thor.Address{}, 0, 0, 0, func(uint32) thor.Hash { return thor.Hash{} })

		gas := uint64(1000000)
		// cost about  49165 gas
		out := rt.Call(
			contracts.Energy.PackConsume(
				thor.BytesToAddress([]byte("acc1")),
				thor.Address{},
				big.NewInt(1),
			),
			0, gas, contracts.Energy.Address, new(big.Int), thor.Hash{})
		if out.VMErr != nil {
			panic(out.VMErr)
		}

		// root, err = st.Stage().Commit()
		// if err != nil {
		// 	panic(err)
		// }
	}
}

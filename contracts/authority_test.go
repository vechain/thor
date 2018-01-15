package contracts_test

import (
	"math/big"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/block"
	. "github.com/vechain/thor/contracts"
	"github.com/vechain/thor/fortest"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/schedule"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

func TestAuthority(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	st.SetCode(Authority.Address, Authority.RuntimeBytecodes())

	rt := runtime.New(st, &block.Header{}, func(uint32) thor.Hash { return thor.Hash{} })

	call := func(data []byte) *vm.Output {
		return rt.Execute(&tx.Clause{
			To:    &Authority.Address,
			Value: &big.Int{},
			Data:  data,
		}, 0, 1000000, Authority.Address, &big.Int{}, thor.Hash{})
	}

	out := call(Authority.PackInitialize(thor.BytesToAddress([]byte("voting"))))
	assert.Nil(t, out.VMErr)

	addr1 := thor.BytesToAddress([]byte("a1"))
	id1 := "I'm a1"

	///// preset
	out = call(Authority.PackPreset(
		addr1,
		id1,
	))
	assert.Nil(t, out.VMErr)

	//// proposer
	out = call(Authority.PackProposer(addr1))
	assert.Nil(t, out.VMErr)

	assert.Equal(t, []interface{}{
		uint32(0),
		id1,
	}, fortest.Multi(Authority.UnpackProposer(out.Value)))

	//// proposers
	out = call(Authority.PackProposers())
	assert.Nil(t, out.VMErr)
	assert.Equal(t, []schedule.Proposer{
		{Address: addr1, Status: 0},
	}, Authority.UnpackProposers(out.Value))
}

func BenchmarkProposers(b *testing.B) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)

	st.SetCode(Authority.Address, Authority.RuntimeBytecodes())
	rt := runtime.New(st, &block.Header{}, func(uint32) thor.Hash { return thor.Hash{} })

	call := func(data []byte) *vm.Output {
		return rt.Execute(&tx.Clause{
			To:    &Authority.Address,
			Value: &big.Int{},
			Data:  data,
		}, 0, 1000000, Authority.Address, &big.Int{}, thor.Hash{})
	}

	for i := 0; i < 100; i++ {
		acc := thor.BytesToAddress([]byte("acc" + strconv.Itoa(i)))
		id := "acc" + strconv.Itoa(i)
		out := call(Authority.PackPreset(acc, id))
		if out.VMErr != nil {
			b.Fatal(out.VMErr)
		}
	}

	// evaluate `proposers` performance
	for i := 0; i < b.N; i++ {
		out := call(Authority.PackProposers())
		if out.VMErr != nil {
			b.Fatal(out.VMErr)
		}
		Authority.UnpackProposers(out.Value)
	}
}

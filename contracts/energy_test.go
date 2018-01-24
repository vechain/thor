package contracts_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	. "github.com/vechain/thor/contracts"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

type account struct {
	addr    thor.Address
	balance *big.Int
}

func TestEnergy(t *testing.T) {
	checkChargeAncConsume(t)
	checkBalanceGrowth(t)
	checkMulBalanceGrowth(t)
	checkTransferBalance(t)
}

func checkChargeAncConsume(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)
	st.SetCode(Energy.Address, Energy.RuntimeBytecodes())

	rt := runtime.New(st,
		thor.Address{}, 0, 1000000, 1000000,
		func(uint32) thor.Hash { return thor.Hash{} })
	call := func(clause *tx.Clause) *vm.Output {
		return rt.Call(
			clause,
			0, 1000000, Energy.Address, &big.Int{}, thor.Hash{})
	}

	balance := big.NewInt(1e18)

	accounts := []account{
		{
			thor.BytesToAddress([]byte("acc1")), new(big.Int).Mul(balance, big.NewInt(10)),
		},
		{
			thor.BytesToAddress([]byte("acc2")), new(big.Int).Mul(balance, big.NewInt(100)),
		},
		{
			thor.BytesToAddress([]byte("acc3")), new(big.Int).Mul(balance, big.NewInt(1000)),
		},
	}

	for _, a := range accounts {
		//gasused 100000-45336
		out := call(Energy.PackCharge(a.addr, a.balance))
		//gasused 100000-65959
		out = call(Energy.PackUpdateBalance(a.addr))
		assert.Equal(t, a.balance, new(big.Int).SetBytes(out.Value))
	}

	for _, a := range accounts {
		rt := runtime.New(st,
			thor.Address{}, 0, 1000000, 1000000,
			func(uint32) thor.Hash { return thor.Hash{} })
		call := func(clause *tx.Clause) *vm.Output {
			return rt.Call(
				clause,
				0, 100000, Energy.Address, &big.Int{}, thor.Hash{})
		}
		consume := new(big.Int).Div(a.balance, big.NewInt(2))
		//gasused 100000-80097
		out := call(Energy.PackConsume(a.addr, a.addr, consume))
		out = call(Energy.PackUpdateBalance(a.addr))
		assert.Equal(t, new(big.Int).Sub(a.balance, consume), new(big.Int).SetBytes(out.Value))
	}

}

func checkBalanceGrowth(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)
	st.SetCode(Energy.Address, Energy.RuntimeBytecodes())
	st.SetCode(Voting.Address, Voting.RuntimeBytecodes())

	rt := runtime.New(st,
		thor.Address{}, 0, 1000000, 1000000,
		func(uint32) thor.Hash { return thor.Hash{} })
	call := func(clause *tx.Clause) *vm.Output {
		return rt.Call(
			clause,
			0, 1000000, Energy.Address, &big.Int{}, thor.Hash{})
	}
	//gasused 100000-978800
	out := call(Energy.PackInitialize(Voting.Address))
	vet := big.NewInt(1e18)
	addr := thor.BytesToAddress([]byte("acc1"))

	st.SetBalance(addr, vet)
	out = call(Energy.PackUpdateBalance(addr))
	assert.Equal(t, int64(0), new(big.Int).SetBytes(out.Value).Int64())

	callFromVoting := func(clause *tx.Clause) *vm.Output {
		return rt.Call(
			clause,
			0, 1000000, Voting.Address, &big.Int{}, thor.Hash{})
	}

	//gasused 100000-936135
	birth := big.NewInt(100)
	timeInterval := uint64(10)
	callFromVoting(Energy.PackSetBalanceBirth(birth))

	rt = runtime.New(st,
		thor.Address{}, 0, 1000000+timeInterval, 1000001,
		func(uint32) thor.Hash { return thor.Hash{} })
	out = call(Energy.PackUpdateBalance(addr))
	assert.Equal(t, new(big.Int).Mul(big.NewInt(int64(timeInterval)), birth), new(big.Int).SetBytes(out.Value))

}

func checkMulBalanceGrowth(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)
	st.SetCode(Energy.Address, Energy.RuntimeBytecodes())
	st.SetCode(Voting.Address, Voting.RuntimeBytecodes())

	rt := runtime.New(st,
		thor.Address{}, 0, 1000000, 1000000,
		func(uint32) thor.Hash { return thor.Hash{} })
	call := func(clause *tx.Clause) *vm.Output {
		return rt.Call(
			clause,
			0, 1000000, Energy.Address, &big.Int{}, thor.Hash{})
	}
	//gasused 100000-978800
	out := call(Energy.PackInitialize(Voting.Address))
	vet := big.NewInt(1e18)
	addr := thor.BytesToAddress([]byte("acc1"))

	st.SetBalance(addr, vet)
	out = call(Energy.PackUpdateBalance(addr))
	assert.Equal(t, int64(0), new(big.Int).SetBytes(out.Value).Int64())
	//initialize birth
	birth := big.NewInt(100)
	callFromVoting := func(clause *tx.Clause) *vm.Output {
		return rt.Call(
			clause,
			0, 1000000, Voting.Address, &big.Int{}, thor.Hash{})
	}
	callFromVoting(Energy.PackSetBalanceBirth(birth))

	//gasused 100000-936135
	//set birth
	time := 0
	timeInterval := 2000
	b := 10
	totalBenefit := 0

	for i := 0; i < 10; i++ {
		rt = runtime.New(st,
			thor.Address{}, 0, 1000000+uint64(time), 1000000,
			func(uint32) thor.Hash { return thor.Hash{} })
		callFromVoting(Energy.PackSetBalanceBirth(big.NewInt(int64(b))))
		time += timeInterval
		totalBenefit += timeInterval * b
		b += 20
	}

	rt = runtime.New(st,
		thor.Address{}, 0, 1000000+uint64(time), 1000000,
		func(uint32) thor.Hash { return thor.Hash{} })
	//gasused 100000-927830
	out = call(Energy.PackUpdateBalance(addr))
	assert.Equal(t, big.NewInt(int64(totalBenefit)), new(big.Int).SetBytes(out.Value))

}

func checkTransferBalance(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)
	st.SetCode(Energy.Address, Energy.RuntimeBytecodes())

	rt := runtime.New(st,
		thor.Address{}, 0, 1000000, 1000000,
		func(uint32) thor.Hash { return thor.Hash{} })
	call := func(clause *tx.Clause) *vm.Output {
		return rt.Call(
			clause,
			0, 1000000, Energy.Address, &big.Int{}, thor.Hash{})
	}

	acc1 := thor.BytesToAddress([]byte("acc1"))
	balance1 := big.NewInt(10000)
	acc2 := thor.BytesToAddress([]byte("acc2"))
	balance2 := big.NewInt(10000)
	transfer := big.NewInt(2000)

	call(Energy.PackCharge(acc1, balance1))
	call(Energy.PackCharge(acc2, balance2))

	callTransfer := func(clause *tx.Clause) *vm.Output {
		return rt.Call(
			clause,
			0, 1000000, acc1, &big.Int{}, thor.Hash{})
	}
	callTransfer(Energy.PackTransfer(acc2, transfer))
	b1 := call(Energy.PackBalanceOf(acc1))
	b2 := call(Energy.PackBalanceOf(acc2))
	assert.Equal(t, new(big.Int).Sub(balance1, transfer), new(big.Int).SetBytes(b1.Value))
	assert.Equal(t, new(big.Int).Add(balance2, transfer), new(big.Int).SetBytes(b2.Value))
}

package contracts_test

import (
	// "fmt"
	"github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/assert"
	. "github.com/vechain/thor/contracts"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
	"math/big"
	"testing"
)

type account struct {
	addr    thor.Address
	balance *big.Int
}

func TestEnergy(t *testing.T) {
	checkChargeAncConsume(t)

}

func checkChargeAncConsume(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)
	st.SetCode(Energy.Address, Energy.RuntimeBytecodes())
	rt := runtime.New(st,
		thor.Address{}, 0, 1000000, 1000000,
		func(uint32) thor.Hash { return thor.Hash{} })
	call := func(data []byte) *vm.Output {
		return rt.Execute(
			tx.NewClause(&Energy.Address).WithData(data),
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
		call(Energy.PackCharge(a.addr, a.balance))
		out := call(Energy.PackBalanceOf(a.addr))
		assert.Equal(t, a.balance, new(big.Int).SetBytes(out.Value))
	}

	for _, a := range accounts {
		consume := new(big.Int).Div(a.balance, big.NewInt(2))
		call(Energy.PackConsume(a.addr, a.addr, consume))
		out := call(Energy.PackBalanceOf(a.addr))
		assert.Equal(t, new(big.Int).Sub(a.balance, consume), new(big.Int).SetBytes(out.Value))
	}

}

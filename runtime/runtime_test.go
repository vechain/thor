package runtime_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/block"

	"github.com/vechain/thor/tx"

	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/vm"
)

func TestRuntime(t *testing.T) {
	kv, _ := lvldb.NewMem()
	state, _ := state.New(cry.Hash{}, kv)

	rt := runtime.New(state, &block.Header{}, func(uint64) cry.Hash { return cry.Hash{} }, vm.Config{})
	output := rt.Exec(&tx.Clause{}, 0, 1000000, acc.BytesToAddress([]byte("acc1")), &big.Int{}, cry.Hash{})
	fmt.Printf("%+v\n", output)
}

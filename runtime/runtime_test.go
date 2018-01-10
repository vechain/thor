package runtime_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestRuntime(t *testing.T) {

	kv, _ := lvldb.NewMem()
	state, _ := state.New(thor.Hash{}, kv)

	rt := runtime.New(state, &block.Header{}, func(uint64) thor.Hash { return thor.Hash{} })
	rt.SetTransactionEnvironment(thor.BytesToAddress([]byte("acc1")), &big.Int{}, thor.Hash{})
	output := rt.Execute(&tx.Clause{}, 0, 1000000)
	fmt.Printf("%+v\n", output)
}

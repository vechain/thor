package fortest_test

import (
	"fmt"
	"testing"

	"github.com/vechain/thor/fortest"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

func TestGenesis(t *testing.T) {
	kv, _ := lvldb.NewMem()
	st, _ := state.New(thor.Hash{}, kv)
	fmt.Println(fortest.BuildGenesis(st))
}

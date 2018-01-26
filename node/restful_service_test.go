package node

import (
	"context"
	"net"
	"testing"

	"github.com/vechain/thor/fortest"

	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
)

func Test_restfulService(t *testing.T) {
	lv, err := lvldb.NewMem()
	if err != nil {
		t.Fatal(err)
	}

	stateC := state.NewCreator(lv).NewState

	genesis, err := makeGenesisBlock(stateC, fortest.BuildGenesis)
	if err != nil {
		t.Fatal(err)
	}

	chain, err := makeChain(lv, genesis)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		t.Fatal(err)
	}

	restfulService(context.Background(), listener, chain, stateC)
}

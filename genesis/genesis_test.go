package genesis_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
)

func TestGenesis(t *testing.T) {
	assert := assert.New(t)
	kv, _ := lvldb.NewMem()
	defer kv.Close()
	b0, err := genesis.Mainnet.Build(state.NewCreator(kv))
	if err != nil {
		t.Fatal(err)
	}

	st, _ := state.New(b0.Header().StateRoot(), kv)
	assert.True(len(st.GetCode(contracts.Authority.Address)) > 0)
}
func TestDevGenesis(t *testing.T) {
	assert := assert.New(t)
	kv, _ := lvldb.NewMem()
	defer kv.Close()
	b0, err := genesis.Dev.Build(state.NewCreator(kv))
	if err != nil {
		t.Fatal(err)
	}

	st, _ := state.New(b0.Header().StateRoot(), kv)
	assert.True(len(st.GetCode(contracts.Authority.Address)) > 0)
	fmt.Println(b0)
}

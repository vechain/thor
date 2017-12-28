package genesis_test

import (
	"testing"

	"github.com/vechain/thor/genesis/contracts"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	State "github.com/vechain/thor/state"
)

func TestGenesis(t *testing.T) {
	assert := assert.New(t)
	kv, _ := lvldb.NewMem()
	defer kv.Close()
	state, _ := State.New(cry.Hash{}, kv)
	block, _ := genesis.Build(state)

	state, _ = State.New(block.Header().StateRoot(), kv)
	assert.True(len(state.GetCode(contracts.Authority.Address)) > 0)
}

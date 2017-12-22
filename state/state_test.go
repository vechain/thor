package state_test

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
)

const (
	emptyRootHash = "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421"
)

func TestState(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	rootHash, _ := cry.ParseHash(emptyRootHash)
	state, _ := state.New(*rootHash, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e01a")
	account := &acc.Account{
		Balance:     new(big.Int),
		CodeHash:    cry.Hash{0xaa, 0x22},
		StorageRoot: cry.Hash{0xaa, 0x22},
	}
	state.Update(*address, account)
	a, _ := state.Get(*address)
	assert.Equal(t, a, account, "account should be equal")
}

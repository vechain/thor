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

func TestBalance(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	rootHash, _ := cry.ParseHash(emptyRootHash)
	stat, _ := state.New(*rootHash, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")
	balance := big.NewInt(100)
	stat.SetBalance(*address, balance)
	b := stat.GetBalance(*address)
	assert.Equal(t, b, balance, "Balance should be equal")
}

func TestCode(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	rootHash, _ := cry.ParseHash(emptyRootHash)
	stat, _ := state.New(*rootHash, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")
	code := []byte{0x10, 0x11}
	stat.SetCode(*address, code)
	c := stat.GetCode(*address)
	assert.Equal(t, c, code, "Code should be equal")
}
func TestStorage(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	rootHash, _ := cry.ParseHash(emptyRootHash)
	stat, _ := state.New(*rootHash, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")
	k1, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b410")
	v1, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b412")
	stat.SetStorage(*address, *k1, *v1)
	stat.Root()
	// s, _ := state.New(r, db)
	v2 := stat.GetStorage(*address, *k1)
	assert.Equal(t, v1.String(), v2.String(), "storage should be equal")
}

func TestDelete(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	rootHash, _ := cry.ParseHash(emptyRootHash)
	stat, _ := state.New(*rootHash, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")
	stat.Delete(*address)
	stat.Root()
}
func TestExist(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	rootHash, _ := cry.ParseHash(emptyRootHash)
	stat, _ := state.New(*rootHash, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")
	isExist := stat.Exists(*address)
	assert.False(t, isExist, "account should not exist in trie")
	stat.SetBalance(*address, big.NewInt(100))
	stat.Root()
	isExist = stat.Exists(*address)
	assert.True(t, isExist, "account should exist in trie")
}

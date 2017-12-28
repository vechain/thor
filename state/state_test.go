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

func TestBalance(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	stat, _ := state.New(cry.Hash{}, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")

	b := stat.GetBalance(*address)
	assert.Equal(t, b, big.NewInt(0), "Balance should be empty")

	balance := big.NewInt(100)
	stat.SetBalance(*address, balance)
	b1 := stat.GetBalance(*address)
	assert.Equal(t, b1, balance, "Balance should be equal")

	hash := stat.Commit()
	newState, _ := state.New(hash, db)
	b2 := newState.GetBalance(*address)
	assert.Equal(t, b2, balance, "Balance should be equal to existing account balance")
}

func TestCode(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	stat, _ := state.New(cry.Hash{}, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")
	code := stat.GetCode(*address)
	assert.Nil(t, code, "initial code should be nil")

	testCode := []byte{0x10, 0x11}
	stat.SetCode(*address, testCode)
	c := stat.GetCode(*address)
	assert.Equal(t, c, testCode, "cached code should be equal to test code")

	hash := stat.Commit()
	newState, _ := state.New(hash, db)
	existCode := newState.GetCode(*address)
	assert.Equal(t, existCode, testCode, "existing code should be equal to test code")

}
func TestStorage(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	stat, _ := state.New(cry.Hash{}, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")
	testKey, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b410")
	testValue, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b412")
	emptyHash := cry.Hash{}

	v := stat.GetStorage(*address, *testKey)
	assert.Equal(t, v.String(), emptyHash.String(), "storage should be empty")

	stat.SetStorage(*address, *testKey, *testValue)
	cachedStorageValue := stat.GetStorage(*address, *testKey)
	assert.Equal(t, cachedStorageValue.String(), testValue.String(), "cached storage value should be equal to test value")

	stat.SetStorage(*address, *testKey, *testValue)
	stat.Root()
	emptyAccountStorageValue := stat.GetStorage(*address, *testKey)
	assert.Equal(t, emptyAccountStorageValue.String(), emptyHash.String(), "empty accoun storage value should be empty")

	stat.SetStorage(*address, *testKey, *testValue)
	stat.SetBalance(*address, big.NewInt(100))
	stat.Root()
	trieStorageValue := stat.GetStorage(*address, *testKey)
	assert.Equal(t, trieStorageValue.String(), testValue.String(), "storage value in trie should be equal to test value")

	stat.SetStorage(*address, *testKey, *testValue)
	stat.SetBalance(*address, big.NewInt(100))
	hash := stat.Commit()
	newState, _ := state.New(hash, db)
	existAccountStorageValue := newState.GetStorage(*address, *testKey)
	assert.Equal(t, existAccountStorageValue.String(), testValue.String(), "exist account storage value should be equal to test value")
}

func TestDelete(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	stat, _ := state.New(cry.Hash{}, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")
	stat.SetBalance(*address, big.NewInt(100))
	isExist := stat.Exists(*address)
	assert.True(t, isExist, "account should be existing")

	hash := stat.Commit()
	newState, _ := state.New(hash, db)
	isExist = newState.Exists(*address)
	assert.True(t, isExist, "account should be existing")

	newState.Delete(*address)
	isExist = newState.Exists(*address)
	assert.False(t, isExist, "account should be not existing")

	notExistAddress, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e091")
	newState.Delete(*notExistAddress)
	isExist = newState.Exists(*notExistAddress)
	assert.False(t, isExist, "account should be not existing")
}
func TestExist(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	stat, _ := state.New(cry.Hash{}, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")

	isExist := stat.Exists(*address)
	assert.False(t, isExist, "account should not exist in trie")
	stat.SetBalance(*address, big.NewInt(100))
	stat.Commit()
	isExist = stat.Exists(*address)
	assert.True(t, isExist, "account should exist in trie")

	stat.SetBalance(*address, new(big.Int))
	stat.Commit()
	isExist = stat.Exists(*address)
	assert.False(t, isExist, "empty account should not exist in trie")
}

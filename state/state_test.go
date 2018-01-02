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
	// get balance from an account that is not existing
	cas := struct {
		want *big.Int
	}{
		big.NewInt(0),
	}
	b := stat.GetBalance(*address)
	assert.Equal(t, b, cas.want, "balance of an account that not exists should be empty")
	//set balance for account and get the same balance from the account
	cases := []struct {
		in, want *big.Int
	}{
		{big.NewInt(0), big.NewInt(0)},
		{big.NewInt(10), big.NewInt(10)},
		{big.NewInt(100), big.NewInt(100)},
	}
	for _, c := range cases {
		stat.SetBalance(*address, c.in)
		got := stat.GetBalance(*address)
		assert.Equal(t, got, c.want, "Balance should be equal")
	}
	//get balance from an existing account
	c := struct {
		in, want *big.Int
	}{
		big.NewInt(1000), big.NewInt(1000),
	}
	stat.SetBalance(*address, c.in)
	hash := stat.Commit()
	newState, _ := state.New(hash, db)
	balance := newState.GetBalance(*address)
	assert.Equal(t, balance, c.want, "Balance should be equal to existing account balance")
}

func TestCode(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	stat, _ := state.New(cry.Hash{}, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")
	//get code from an account that is not existing
	code := stat.GetCode(*address)
	assert.Nil(t, code, "initial code should be nil")
	//set code for account and get the same code from the account
	cases := []struct {
		in, want []byte
	}{
		{[]byte{}, []byte{}},
		{[]byte{0x00, 0x01, 0x02, 0x03}, []byte{0x00, 0x01, 0x02, 0x03}},
		{[]byte{0x00, 0x01, 0x02, 0x03, 0x00, 0x01, 0x02, 0x03}, []byte{0x00, 0x01, 0x02, 0x03, 0x00, 0x01, 0x02, 0x03}},
	}
	for _, c := range cases {
		stat.SetCode(*address, c.in)
		got := stat.GetCode(*address)
		assert.Equal(t, got, c.want, "cached code should be equal to test code")
	}
	//get code from an existing account
	c := struct {
		in, want []byte
	}{
		[]byte{0x00, 0x01, 0x02, 0x03, 0x04},
		[]byte{0x00, 0x01, 0x02, 0x03, 0x04},
	}
	stat.SetCode(*address, c.in)
	hash := stat.Commit()
	newState, _ := state.New(hash, db)
	existCode := newState.GetCode(*address)
	assert.Equal(t, existCode, c.want, "existing code should be equal to test code")

}
func TestStorage(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	stat, _ := state.New(cry.Hash{}, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")
	emptyHash := cry.Hash{}
	k1, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b410")
	v1, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b411")
	k2, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b412")
	v2, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b413")
	k3, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b414")
	v3, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b415")
	v := stat.GetStorage(*address, *k1)
	assert.Equal(t, v.String(), emptyHash.String(), "storage should be empty")
	//set storaget for account and get the same storage from the account
	cases := struct {
		in, want map[cry.Hash]cry.Hash
	}{
		map[cry.Hash]cry.Hash{
			*k1: *v1,
			*k2: *v2,
			*k3: *v3,
		},
		map[cry.Hash]cry.Hash{
			*k1: *v1,
			*k2: *v2,
			*k3: *v3,
		},
	}
	for key, value := range cases.in {
		stat.SetStorage(*address, key, value)
		cachedStorageValue := stat.GetStorage(*address, key)
		assert.Equal(t, cachedStorageValue.String(), cases.want[key].String(), "cached storage value should be equal to test value")
	}

	stat.SetStorage(*address, *k1, *v1)
	stat.Root()
	accountStorageValue := stat.GetStorage(*address, *k1)
	assert.Equal(t, accountStorageValue.String(), v1.String(), "storage update to trie , return the same value")

	stat.SetStorage(*address, *k1, *v1)
	stat.SetBalance(*address, big.NewInt(100))
	stat.Root()
	trieStorageValue := stat.GetStorage(*address, *k1)
	assert.Equal(t, trieStorageValue.String(), v1.String(), "storage value in trie should be equal to test value")

	stat.SetStorage(*address, *k1, *v1)
	stat.SetBalance(*address, big.NewInt(100))
	hash := stat.Commit()
	newState, _ := state.New(hash, db)
	existAccountStorageValue := newState.GetStorage(*address, *k1)
	assert.Equal(t, existAccountStorageValue.String(), v1.String(), "exist account storage value should be equal to test value")
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

	stat.SetBalance(*address, big.NewInt(0))
	stat.Commit()
	isExist = stat.Exists(*address)
	assert.False(t, isExist, "empty account should not exist in trie")
}

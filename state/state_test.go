package state_test

import (
	"fmt"
	"math/big"
	"testing"

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
	hash, _ := cry.ParseHash(emptyRootHash)
	state, _ := state.New(*hash, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e01a")
	account := &acc.Account{
		Balance:     new(big.Int),
		CodeHash:    cry.Hash{0xaa, 0x22},
		StorageRoot: cry.Hash{0xaa, 0x22},
	}
	state.UpdateAccount(*address, account)
	a := state.GetAccount(*address)
	account = &acc.Account{
		Balance:     new(big.Int),
		CodeHash:    cry.Hash{0xaa},
		StorageRoot: cry.Hash{0xaa},
	}
	state.UpdateAccount(*address, account)
	fmt.Printf("3 acc %v\n root %v\n:", a, state.Hash().String())
	a = state.GetAccount(*address)
	fmt.Printf("4 acc %v\n root %v\n state %v \n: ", a, state.Hash().String(), *state)
	root, err := state.Commit()
	fmt.Printf("5 root %v\n err %v \n rootHash %v\n:", root, err, state.Hash().String())
}

func TestStorage(t *testing.T) {
	db, _ := lvldb.NewMem()
	defer db.Close()
	storage := state.NewStorage(db)
	hash, _ := cry.ParseHash(emptyRootHash)
	fmt.Printf("hash %v", hash)
	storage.UpdateStorage(*hash, *hash, *hash)
	v := storage.GetStorage(*hash, *hash)
	fmt.Printf("\nv %v\n", v)
}

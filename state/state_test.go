package state_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/vechain/vecore/acc"
	"github.com/vechain/vecore/cry"
	"github.com/vechain/vecore/kv"
	"github.com/vechain/vecore/state"
)

const (
	emptyRootHash = "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421"
)

func TestState(t *testing.T) {
	opt := kv.Options{CacheSize: 10, OpenFilesCacheCapacity: 10}
	db, _ := kv.New("/Users/dinn/Desktop/db", opt)
	// hash, _ := cry.ParseHash(emptyRootHash)
	hash, _ := cry.ParseHash("0xcfcc4b2abe6c249cbb48466ef89e949e4950c75f98a739b4a079d8d84a9593f5")
	state, _ := state.NewState(*hash, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e01a")
	account := &acc.Account{
		Balance:     new(big.Int),
		CodeHash:    cry.Hash{0xaa, 0x22},
		StorageRoot: cry.Hash{0xaa, 0x22},
	}
	fmt.Printf("*address %v\n  *account %v \n:", address, account)
	state.UpdateState(*address, account)
	a := state.GetAccount(*address)
	fmt.Printf("acc %v\n root %v\n:", a, state.Hash().String())
	account = &acc.Account{
		Balance:     new(big.Int),
		CodeHash:    cry.Hash{0xaa},
		StorageRoot: cry.Hash{0xaa},
	}
	state.UpdateState(*address, account)
	a = state.GetAccount(*address)
	fmt.Printf("acc %v\n root %v\n:", a, state.Hash().String())
	root, err := state.Commit()
	fmt.Printf("root %v\n err %v \n rootHash %v\n:", root, err, state.Hash().String())
}

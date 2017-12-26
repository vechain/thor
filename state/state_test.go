package state_test

import (
	"fmt"
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
	// db, _ := lvldb.NewMem()
	opt := lvldb.Options{CacheSize: 10, OpenFilesCacheCapacity: 10}
	db, _ := lvldb.New("/Users/dinn/Desktop/db", opt)
	defer db.Close()
	rootHash, _ := cry.ParseHash(emptyRootHash)
	stat, _ := state.New(*rootHash, db)
	address, _ := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e090")
	stat.SetBalance(*address, big.NewInt(10))
	stat.SetCode(*address, []byte{0x11, 0x12, 0x13, 0x14})
	k1, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b410")
	k2, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b411")
	v1, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b412")
	v2, _ := cry.ParseHash("56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b413")
	stat.SetStorage(*address, *k1, *v1)
	stat.SetStorage(*address, *k2, *v2)
	stat.Commit()
	r := stat.Root()
	s, _ := state.New(r, db)
	v3 := s.GetStorage(*address, *k1)
	v4 := s.GetStorage(*address, *k2)
	cod := s.GetCode(*address)
	assert.Equal(t, v1.String(), v3.String(), "storage should be equal")
	fmt.Println("v3", v3, v4, cod)
}

package main

import (
	"fmt"
	"math/big"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
)

const (
	emptyRootHash = "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421"
	testAddress   = "56e81f171bcc55a6ff8345e692c0f86e5b48e01a"
)

func main() {

	opt := lvldb.Options{CacheSize: 10, OpenFilesCacheCapacity: 10}
	db, _ := lvldb.New("/Users/dinn/Desktop/db", opt)
	hash, _ := cry.ParseHash(emptyRootHash)
	s, _ := state.New(*hash, db)
	address, _ := acc.ParseAddress(testAddress)
	account := &acc.Account{
		Balance:     new(big.Int),
		CodeHash:    cry.Hash{0xaa, 0x22},
		StorageRoot: cry.Hash{0xaa, 0x22},
	}
	s.UpdateAccount(*address, account)
	account = &acc.Account{
		Balance:     new(big.Int),
		CodeHash:    cry.Hash{0xaa},
		StorageRoot: cry.Hash{0xaa},
	}
	s.UpdateAccount(*address, account)
	s.Commit()
	chain := chain.New(db)
	chain.WriteGenesis(new(block.Builder).Build())

	for i := 0; i < 100; i++ {
		best, _ := chain.GetBestBlock()
		b := new(block.Builder).
			ParentHash(best.Hash()).
			Build()
		fmt.Println(b.Hash())
		if err := chain.AddBlock(b, true); err != nil {
			fmt.Println(err)
		}
	}
	best, _ := chain.GetBestBlock()
	fmt.Println(best.Number())
	bm := api.NewBlockMananger(chain)
	am := api.NewAccountMananger(s)
	router := mux.NewRouter()
	api.NewAccountHTTPRouter(router, am)
	api.NewBlockHTTPRouter(router, bm)
	fmt.Println("server listen 3000")
	http.ListenAndServe(":3000", router)

}

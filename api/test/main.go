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
	db, _ := lvldb.NewMem()
	hash, _ := cry.ParseHash(emptyRootHash)
	s, _ := state.New(*hash, db)
	address, _ := acc.ParseAddress(testAddress)
	s.SetBalance(*address, big.NewInt(100))
	s.SetCode(*address, []byte{0x11, 0x12})
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
	ai := api.NewAccountInterface(s)
	bi := api.NewBlockInterface(chain)
	router := mux.NewRouter()
	api.NewAccountHTTPRouter(router, ai)
	api.NewBlockHTTPRouter(router, bi)
	fmt.Println("server listen 3000")
	http.ListenAndServe(":3000", router)

}

package main

import (
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"math/big"
	"net/http"
)

const (
	emptyRootHash = "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421"
	testAddress   = "56e81f171bcc55a6ff8345e692c0f86e5b48e01a"
)

func main() {
	db, _ := lvldb.NewMem()
	hash, _ := thor.ParseHash(emptyRootHash)
	s, _ := state.New(hash, db)
	address, _ := thor.ParseAddress(testAddress)
	s.SetBalance(address, big.NewInt(100))
	s.SetCode(address, []byte{0x11, 0x12})
	chain := chain.New(db)
	b, err := genesis.Build(s)
	if err != nil {
		fmt.Println(err)
	}
	chain.WriteGenesis(b)
	key, _ := crypto.GenerateKey()
	for i := 0; i < 100; i++ {
		best, _ := chain.GetBestBlock()
		cla := &tx.Clause{To: &address, Value: big.NewInt(10), Data: nil}
		signing := cry.NewSigning(best.Hash())
		tx1 := new(tx.Builder).
			GasPrice(big.NewInt(1000)).
			Gas(1000).
			TimeBarrier(10000).
			Clause(cla).
			Nonce(1).
			Build()

		sig, _ := signing.Sign(tx1, crypto.FromECDSA(key))
		tx1 = tx1.WithSignature(sig)

		tx2 := new(tx.Builder).
			GasPrice(big.NewInt(10000)).
			Gas(10000).
			TimeBarrier(100000).
			Clause(cla).
			Nonce(2).
			Build()

		sig, _ = signing.Sign(tx2, crypto.FromECDSA(key))
		tx2 = tx2.WithSignature(sig)

		b := new(block.Builder).
			ParentHash(best.Hash()).
			Transaction(tx1).
			Transaction(tx2).
			Build()
		fmt.Println(b.Hash())
		if err := chain.AddBlock(b, true); err != nil {
			fmt.Println(err)
		}

	}
	s.Stage().Commit()

	ai := api.NewAccountInterface(s)
	bi := api.NewBlockInterface(chain)
	ti := api.NewTransactionInterface(chain)
	router := mux.NewRouter()
	api.NewAccountHTTPRouter(router, ai)
	api.NewBlockHTTPRouter(router, bi)
	api.NewTransactionHTTPRouter(router, ti)
	fmt.Println("server listen 3000")
	http.ListenAndServe(":3000", router)

}

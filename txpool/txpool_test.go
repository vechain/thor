package txpool_test

import (
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
	"math/big"
	"testing"
)

const (
	emptyRootHash = "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421"
	testAddress   = "56e81f171bcc55a6ff8345e692c0f86e5b48e01a"
	testPrivHex   = "289c2857d4598e37fb9647507e47a309d6133539bf21a8b9cb6df88fd5232032"
)

func BenchmarkAddTx(b *testing.B) {
	db, _ := lvldb.NewMem()
	chain := chain.New(db)
	c := state.NewCreator(db)
	bl, err := genesis.Mainnet.Build(c)
	if err != nil {
		b.Fatal(err)
	}
	chain.WriteGenesis(bl)
	address, _ := thor.ParseAddress(testAddress)
	pool := txpool.NewTxPool(chain)
	for i := 0; i < 2000; i++ {
		cla := tx.NewClause(&address).WithValue(big.NewInt(10 + int64(i))).WithData(nil)
		tx := new(tx.Builder).
			GasPrice(big.NewInt(1000)).
			Gas(1000 + uint64(i)).
			Clause(cla).
			Nonce(1).
			Build()
		key, err := crypto.HexToECDSA(testPrivHex)
		if err != nil {
			b.Fatal(err)
		}
		sig, err := crypto.Sign(tx.SigningHash().Bytes(), key)
		if err != nil {
			b.Errorf("Sign error: %s", err)
		}
		tx = tx.WithSignature(sig)
		pool.Add(tx)
	}
}

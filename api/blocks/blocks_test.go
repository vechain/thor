package blocks_test

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/api/blocks"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/txpool"
)

const (
	testAddress = "56e81f171bcc55a6ff8345e692c0f86e5b48e01a"
	testPrivHex = "efa321f290811731036e5eccd373114e5186d9fe419081f5a607231279d5ef01"
)

func TestBlock(t *testing.T) {

	block, ts := initBlockServer(t)
	raw, _ := blocks.ConvertBlock(block)
	defer ts.Close()

	res := httpGet(t, ts.URL+"/blocks/"+block.Header().ID().String())
	rb := new(blocks.Block)
	if err := json.Unmarshal(res, &rb); err != nil {
		t.Fatal(err)
	}
	checkBlock(t, raw, rb)
	// get block info with blocknumber
	res = httpGet(t, ts.URL+"/blocks/1")
	if err := json.Unmarshal(res, &rb); err != nil {
		t.Fatal(err)
	}

	checkBlock(t, raw, rb)
	res = httpGet(t, ts.URL+"/blocks/best")
	if err := json.Unmarshal(res, &rb); err != nil {
		t.Fatal(err)
	}
	checkBlock(t, raw, rb)

}

func initBlockServer(t *testing.T) (*block.Block, *httptest.Server) {
	db, _ := lvldb.NewMem()
	stateC := state.NewCreator(db)
	gene, err := genesis.NewDevnet()
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := gene.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	chain, _ := chain.New(db, b)
	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	tx := new(tx.Builder).
		ChainTag(chain.Tag()).
		GasPriceCoef(1).
		Expiration(10).
		Gas(21000).
		Nonce(1).
		Clause(cla).
		BlockRef(tx.NewBlockRef(0)).
		Build()

	sig, err := crypto.Sign(tx.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	tx = tx.WithSignature(sig)
	packer := packer.New(chain, stateC, genesis.DevAccounts()[0].Address, genesis.DevAccounts()[0].Address)
	_, flow, err := packer.Schedule(b.Header(), uint64(time.Now().Unix()))
	err = flow.Adopt(tx)
	if err != nil {
		t.Fatal(err)
	}
	b, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := chain.AddBlock(b, receipts, true); err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	pool := txpool.New(chain, stateC)
	defer pool.Shutdown()
	blocks.New(chain).Mount(router, "/blocks")
	ts := httptest.NewServer(router)
	return b, ts
}

func checkBlock(t *testing.T, expBl *blocks.Block, actBl *blocks.Block) {
	assert.Equal(t, expBl.Number, actBl.Number, "Number should be equal")
	assert.Equal(t, expBl.ID, actBl.ID, "Hash should be equal")
	assert.Equal(t, expBl.ParentID, actBl.ParentID, "ParentID should be equal")
	assert.Equal(t, expBl.Timestamp, actBl.Timestamp, "Timestamp should be equal")
	assert.Equal(t, expBl.TotalScore, actBl.TotalScore, "TotalScore should be equal")
	assert.Equal(t, expBl.GasLimit, actBl.GasLimit, "GasLimit should be equal")
	assert.Equal(t, expBl.GasUsed, actBl.GasUsed, "GasUsed should be equal")
	assert.Equal(t, expBl.Beneficiary, actBl.Beneficiary, "Beneficiary should be equal")
	assert.Equal(t, expBl.TxsRoot, actBl.TxsRoot, "TxsRoot should be equal")
	assert.Equal(t, expBl.StateRoot, actBl.StateRoot, "StateRoot should be equal")
	assert.Equal(t, expBl.ReceiptsRoot, actBl.ReceiptsRoot, "ReceiptsRoot should be equal")
	for i, txhash := range expBl.Transactions {
		assert.Equal(t, txhash, actBl.Transactions[i], "tx hash should be equal")
	}

}

func httpGet(t *testing.T, url string) []byte {
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

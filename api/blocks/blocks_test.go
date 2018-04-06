package blocks_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/api/blocks"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const (
	emptyRootHash = "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421"
	testAddress   = "56e81f171bcc55a6ff8345e692c0f86e5b48e01a"
	testPrivHex   = "efa321f290811731036e5eccd373114e5186d9fe419081f5a607231279d5ef01"
)

func TestBlock(t *testing.T) {

	block, ts := initBlockServer(t)
	raw := blocks.ConvertBlock(block)
	defer ts.Close()

	res := httpGet(t, ts.URL+fmt.Sprintf("/blocks/%v", block.Header().ID()))
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
	b, _, err := genesis.Dev.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}
	chain, _ := chain.New(db, b)
	router := mux.NewRouter()
	blocks.New(chain).Mount(router, "/blocks")
	ts := httptest.NewServer(router)

	address, _ := thor.ParseAddress(testAddress)

	cla := tx.NewClause(&address).WithData(nil).WithValue(big.NewInt(10))
	tx := new(tx.Builder).
		GasPriceCoef(1).
		Gas(1000).
		Clause(cla).
		Nonce(1).
		Build()

	key, err := crypto.HexToECDSA(testPrivHex)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(tx.SigningHash().Bytes(), key)
	if err != nil {
		t.Errorf("Sign error: %s", err)
	}
	tx = tx.WithSignature(sig)

	best, _ := chain.GetBestBlock()
	bl := new(block.Builder).
		ParentID(best.Header().ID()).
		Transaction(tx).
		Build()
	if _, err := chain.AddBlock(bl, nil, true); err != nil {
		t.Fatal(err)
	}

	return bl, ts
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
	for i, txhash := range expBl.Txs {
		assert.Equal(t, txhash, actBl.Txs[i], "tx hash should be equal")
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

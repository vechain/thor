package api_test

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/api"
	"github.com/vechain/thor/api/utils/types"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBlock(t *testing.T) {

	block, ts := addBlock(t)
	raw := types.ConvertBlock(block)
	defer ts.Close()

	res, err := http.Get(ts.URL + fmt.Sprintf("/blocks/%v", block.Header().ID().String()))
	if err != nil {
		t.Fatal(err)
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	rb := new(types.Block)
	if err := json.Unmarshal(r, &rb); err != nil {
		t.Fatal(err)
	}

	checkBlock(t, raw, rb)

	// get transaction from blocknumber with index
	res, err = http.Get(ts.URL + "/blocks?number=1")
	if err != nil {
		t.Fatal(err)
	}
	r, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(r))
	rb = new(types.Block)
	if err := json.Unmarshal(r, &rb); err != nil {
		t.Fatal(err)
	}

	checkBlock(t, raw, rb)

}

func addBlock(t *testing.T) (*block.Block, *httptest.Server) {
	db, _ := lvldb.NewMem()
	chain := chain.New(db)
	bi := api.NewBlockInterface(chain)
	router := mux.NewRouter()
	api.NewBlockHTTPRouter(router, bi)
	fmt.Println(bi, router)
	ts := httptest.NewServer(router)

	stateC := state.NewCreator(db)
	b, err := genesis.Dev.Build(stateC)
	if err != nil {
		t.Fatal(err)
	}

	chain.WriteGenesis(b)
	address, _ := thor.ParseAddress(testAddress)

	cla := tx.NewClause(&address).WithData(nil).WithValue(big.NewInt(10))
	tx := new(tx.Builder).
		GasPrice(big.NewInt(1000)).
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
	if err := chain.AddBlock(bl, true); err != nil {
		t.Fatal(err)
	}

	return bl, ts
}

func checkBlock(t *testing.T, expBl *types.Block, actBl *types.Block) {
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

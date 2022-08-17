// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks

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
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/cmd/thor/solo"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/packer"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

const (
	testAddress = "56e81f171bcc55a6ff8345e692c0f86e5b48e01a"
	testPrivHex = "efa321f290811731036e5eccd373114e5186d9fe419081f5a607231279d5ef01"
)

var blk *block.Block
var ts *httptest.Server

var invalidBytes32 = "0x000000000000000000000000000000000000000000000000000000000000000g" //invlaid bytes32
var invalidNumberRevision = "4294967296"                                                  //invalid block number

func TestBlock(t *testing.T) {
	initBlockServer(t)
	defer ts.Close()
	//invalid block id
	res, statusCode := httpGet(t, ts.URL+"/blocks/"+invalidBytes32)
	assert.Equal(t, http.StatusBadRequest, statusCode)
	//invalid block number
	res, statusCode = httpGet(t, ts.URL+"/blocks/"+invalidNumberRevision)
	assert.Equal(t, http.StatusBadRequest, statusCode)

	res, statusCode = httpGet(t, ts.URL+"/blocks/"+blk.Header().ID().String())
	rb := new(JSONCollapsedBlock)
	if err := json.Unmarshal(res, rb); err != nil {
		t.Fatal(err)
	}
	checkBlock(t, blk, rb)
	assert.Equal(t, http.StatusOK, statusCode)

	res, statusCode = httpGet(t, ts.URL+"/blocks/1")
	if err := json.Unmarshal(res, &rb); err != nil {
		t.Fatal(err)
	}
	checkBlock(t, blk, rb)
	assert.Equal(t, http.StatusOK, statusCode)

	res, statusCode = httpGet(t, ts.URL+"/blocks/best")
	if err := json.Unmarshal(res, &rb); err != nil {
		t.Fatal(err)
	}
	checkBlock(t, blk, rb)
	assert.Equal(t, http.StatusOK, statusCode)

}

func initBlockServer(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := chain.NewRepository(db, b)
	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	tx := new(tx.Builder).
		ChainTag(repo.ChainTag()).
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
	packer := packer.New(repo, stater, genesis.DevAccounts()[0].Address, &genesis.DevAccounts()[0].Address, thor.NoFork)
	sum, _ := repo.GetBlockSummary(b.Header().ID())
	flow, err := packer.Schedule(sum, uint64(time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	err = flow.Adopt(tx)
	if err != nil {
		t.Fatal(err)
	}
	block, stage, receipts, err := flow.Pack(genesis.DevAccounts()[0].PrivateKey, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddBlock(block, receipts, 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetBestBlockID(block.Header().ID()); err != nil {
		t.Fatal(err)
	}
	router := mux.NewRouter()
	New(repo, &solo.BFTEngine{}).Mount(router, "/blocks")
	ts = httptest.NewServer(router)
	blk = block
}

func checkBlock(t *testing.T, expBl *block.Block, actBl *JSONCollapsedBlock) {
	header := expBl.Header()
	assert.Equal(t, header.Number(), actBl.Number, "Number should be equal")
	assert.Equal(t, header.ID(), actBl.ID, "Hash should be equal")
	assert.Equal(t, header.ParentID(), actBl.ParentID, "ParentID should be equal")
	assert.Equal(t, header.Timestamp(), actBl.Timestamp, "Timestamp should be equal")
	assert.Equal(t, header.TotalScore(), actBl.TotalScore, "TotalScore should be equal")
	assert.Equal(t, header.GasLimit(), actBl.GasLimit, "GasLimit should be equal")
	assert.Equal(t, header.GasUsed(), actBl.GasUsed, "GasUsed should be equal")
	assert.Equal(t, header.Beneficiary(), actBl.Beneficiary, "Beneficiary should be equal")
	assert.Equal(t, header.TxsRoot(), actBl.TxsRoot, "TxsRoot should be equal")
	assert.Equal(t, header.StateRoot(), actBl.StateRoot, "StateRoot should be equal")
	assert.Equal(t, header.ReceiptsRoot(), actBl.ReceiptsRoot, "ReceiptsRoot should be equal")
	for i, tx := range expBl.Transactions() {
		assert.Equal(t, tx.ID(), actBl.Transactions[i], "txid should be equal")
	}

}

func httpGet(t *testing.T, url string) ([]byte, int) {
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r, res.StatusCode
}

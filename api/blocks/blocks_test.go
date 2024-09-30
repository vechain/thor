// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks_test

import (
	"encoding/json"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/solo"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

var genesisBlock *block.Block
var blk *block.Block
var ts *httptest.Server

var invalidBytes32 = "0x000000000000000000000000000000000000000000000000000000000000000g" //invlaid bytes32

func TestBlock(t *testing.T) {
	initBlockServer(t)
	defer ts.Close()

	for name, tt := range map[string]func(*testing.T){
		"testBadQueryParams":                    testBadQueryParams,
		"testInvalidBlockID":                    testInvalidBlockID,
		"testInvalidBlockNumber":                testInvalidBlockNumber,
		"testGetBlockByID":                      testGetBlockByID,
		"testGetBlockNotFound":                  testGetBlockNotFound,
		"testGetExpandedBlockByID":              testGetExpandedBlockByID,
		"testGetBlockByHeight":                  testGetBlockByHeight,
		"testGetBestBlock":                      testGetBestBlock,
		"testGetFinalizedBlock":                 testGetFinalizedBlock,
		"testGetJustifiedBlock":                 testGetJustifiedBlock,
		"testGetBlockWithRevisionNumberTooHigh": testGetBlockWithRevisionNumberTooHigh,
	} {
		t.Run(name, tt)
	}
}

func testBadQueryParams(t *testing.T) {
	badQueryParams := "?expanded=1"
	res, statusCode := httpGet(t, ts.URL+"/blocks/best"+badQueryParams)

	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Equal(t, "expanded: should be boolean", strings.TrimSpace(string(res)))
}

func testGetBestBlock(t *testing.T) {
	res, statusCode := httpGet(t, ts.URL+"/blocks/best")
	rb := new(blocks.JSONCollapsedBlock)
	if err := json.Unmarshal(res, &rb); err != nil {
		t.Fatal(err)
	}
	checkCollapsedBlock(t, blk, rb)
	assert.Equal(t, http.StatusOK, statusCode)
}

func testGetBlockByHeight(t *testing.T) {
	res, statusCode := httpGet(t, ts.URL+"/blocks/1")
	rb := new(blocks.JSONCollapsedBlock)
	if err := json.Unmarshal(res, &rb); err != nil {
		t.Fatal(err)
	}
	checkCollapsedBlock(t, blk, rb)
	assert.Equal(t, http.StatusOK, statusCode)
}

func testGetFinalizedBlock(t *testing.T) {
	res, statusCode := httpGet(t, ts.URL+"/blocks/finalized")
	finalized := new(blocks.JSONCollapsedBlock)
	if err := json.Unmarshal(res, &finalized); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, http.StatusOK, statusCode)
	assert.True(t, finalized.IsFinalized)
	assert.Equal(t, uint32(0), finalized.Number)
	assert.Equal(t, genesisBlock.Header().ID(), finalized.ID)
}

func testGetJustifiedBlock(t *testing.T) {
	res, statusCode := httpGet(t, ts.URL+"/blocks/justified")
	justified := new(blocks.JSONCollapsedBlock)
	require.NoError(t, json.Unmarshal(res, &justified))

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, uint32(0), justified.Number)
	assert.Equal(t, genesisBlock.Header().ID(), justified.ID)
}

func testGetBlockByID(t *testing.T) {
	res, statusCode := httpGet(t, ts.URL+"/blocks/"+blk.Header().ID().String())
	rb := new(blocks.JSONCollapsedBlock)
	if err := json.Unmarshal(res, rb); err != nil {
		t.Fatal(err)
	}
	checkCollapsedBlock(t, blk, rb)
	assert.Equal(t, http.StatusOK, statusCode)
}

func testGetBlockNotFound(t *testing.T) {
	res, statusCode := httpGet(t, ts.URL+"/blocks/0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a")

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "null", strings.TrimSpace(string(res)))
}

func testGetExpandedBlockByID(t *testing.T) {
	res, statusCode := httpGet(t, ts.URL+"/blocks/"+blk.Header().ID().String()+"?expanded=true")
	rb := new(blocks.JSONExpandedBlock)
	if err := json.Unmarshal(res, rb); err != nil {
		t.Fatal(err)
	}
	checkExpandedBlock(t, blk, rb)
	assert.Equal(t, http.StatusOK, statusCode)
}

func testInvalidBlockNumber(t *testing.T) {
	invalidNumberRevision := "4294967296" //invalid block number
	_, statusCode := httpGet(t, ts.URL+"/blocks/"+invalidNumberRevision)
	assert.Equal(t, http.StatusBadRequest, statusCode)
}

func testInvalidBlockID(t *testing.T) {
	_, statusCode := httpGet(t, ts.URL+"/blocks/"+invalidBytes32)
	assert.Equal(t, http.StatusBadRequest, statusCode)
}

func testGetBlockWithRevisionNumberTooHigh(t *testing.T) {
	revisionNumberTooHigh := strconv.FormatUint(math.MaxUint64, 10)
	res, statusCode := httpGet(t, ts.URL+"/blocks/"+revisionNumberTooHigh)

	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Equal(t, "revision: block number out of max uint32", strings.TrimSpace(string(res)))
}

func initBlockServer(t *testing.T) {
	db := muxdb.NewMem()
	stater := state.NewStater(db)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}
	genesisBlock = b

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
	bftEngine := solo.NewBFTEngine(repo)
	blocks.New(repo, bftEngine).Mount(router, "/blocks")
	ts = httptest.NewServer(router)
	blk = block
}

func checkCollapsedBlock(t *testing.T, expBl *block.Block, actBl *blocks.JSONCollapsedBlock) {
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

func checkExpandedBlock(t *testing.T, expBl *block.Block, actBl *blocks.JSONExpandedBlock) {
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
		assert.Equal(t, tx.ID(), actBl.Transactions[i].ID, "txid should be equal")
	}
}

func httpGet(t *testing.T, url string) ([]byte, int) {
	res, err := http.Get(url) // nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	r, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return r, res.StatusCode
}

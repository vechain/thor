// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks_test

import (
	"encoding/json"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

const (
	invalidBytes32 = "0x000000000000000000000000000000000000000000000000000000000000000g" // invalid bytes32
)

var (
	genesisBlock *block.Block
	blk          *block.Block
	ts           *httptest.Server
	tclient      *thorclient.Client
)

func TestBlock(t *testing.T) {
	initBlockServer(t)
	defer ts.Close()

	tclient = thorclient.New(ts.URL)
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
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/blocks/best" + badQueryParams)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Equal(t, "expanded: should be boolean", strings.TrimSpace(string(res)))
}

func testGetBestBlock(t *testing.T) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/blocks/best")
	require.NoError(t, err)
	rb := new(blocks.JSONCollapsedBlock)
	if err := json.Unmarshal(res, &rb); err != nil {
		t.Fatal(err)
	}
	checkCollapsedBlock(t, blk, rb)
	assert.Equal(t, http.StatusOK, statusCode)
}

func testGetBlockByHeight(t *testing.T) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/blocks/1")
	require.NoError(t, err)
	rb := new(blocks.JSONCollapsedBlock)
	if err := json.Unmarshal(res, &rb); err != nil {
		t.Fatal(err)
	}
	checkCollapsedBlock(t, blk, rb)
	assert.Equal(t, http.StatusOK, statusCode)
}

func testGetFinalizedBlock(t *testing.T) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/blocks/finalized")
	require.NoError(t, err)
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
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/blocks/justified")
	require.NoError(t, err)
	justified := new(blocks.JSONCollapsedBlock)
	require.NoError(t, json.Unmarshal(res, &justified))

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, uint32(0), justified.Number)
	assert.Equal(t, genesisBlock.Header().ID(), justified.ID)
}

func testGetBlockByID(t *testing.T) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/blocks/" + blk.Header().ID().String())
	require.NoError(t, err)
	rb := new(blocks.JSONCollapsedBlock)
	if err := json.Unmarshal(res, rb); err != nil {
		t.Fatal(err)
	}
	checkCollapsedBlock(t, blk, rb)
	assert.Equal(t, http.StatusOK, statusCode)
}

func testGetBlockNotFound(t *testing.T) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/blocks/0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a")
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "null", strings.TrimSpace(string(res)))
}

func testGetExpandedBlockByID(t *testing.T) {
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/blocks/" + blk.Header().ID().String() + "?expanded=true")
	require.NoError(t, err)

	rb := new(blocks.JSONExpandedBlock)
	if err := json.Unmarshal(res, rb); err != nil {
		t.Fatal(err)
	}
	checkExpandedBlock(t, blk, rb)
	assert.Equal(t, http.StatusOK, statusCode)
}

func testInvalidBlockNumber(t *testing.T) {
	invalidNumberRevision := "4294967296" //invalid block number
	_, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/blocks/" + invalidNumberRevision)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
}

func testInvalidBlockID(t *testing.T) {
	_, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/blocks/" + invalidBytes32)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
}

func testGetBlockWithRevisionNumberTooHigh(t *testing.T) {
	revisionNumberTooHigh := strconv.FormatUint(math.MaxUint64, 10)
	res, statusCode, err := tclient.RawHTTPClient().RawHTTPGet("/blocks/" + revisionNumberTooHigh)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Equal(t, "revision: block number out of max uint32", strings.TrimSpace(string(res)))
}

func initBlockServer(t *testing.T) {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	addr := thor.BytesToAddress([]byte("to"))
	cla := tx.NewClause(&addr).WithValue(big.NewInt(10000))
	trx := tx.MustSign(
		new(tx.Builder).
			ChainTag(thorChain.Repo().ChainTag()).
			GasPriceCoef(1).
			Expiration(10).
			Gas(21000).
			Nonce(1).
			Clause(cla).
			BlockRef(tx.NewBlockRef(0)).
			Build(),
		genesis.DevAccounts()[0].PrivateKey,
	)

	require.NoError(t, thorChain.MintTransactions(genesis.DevAccounts()[0], trx))

	allBlocks, err := thorChain.GetAllBlocks()
	require.NoError(t, err)

	genesisBlock = allBlocks[0]
	blk = allBlocks[1]

	router := mux.NewRouter()
	blocks.New(thorChain.Repo(), thorChain.Engine()).Mount(router, "/blocks")
	ts = httptest.NewServer(router)
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

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transfers_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/api/transfers"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const defaultLogLimit uint64 = 1000

var ts *httptest.Server

func TestEmptyTransfers(t *testing.T) {
	db := createDb(t)
	initTransferServer(t, db, defaultLogLimit)
	defer ts.Close()

	testTransferBadRequest(t)
	testTransferWithEmptyDb(t)
}

func TestTransfers(t *testing.T) {
	db := createDb(t)
	initTransferServer(t, db, defaultLogLimit)
	defer ts.Close()

	blocksToInsert := 5
	insertBlocks(t, db, blocksToInsert)

	testTransferWithBlocks(t, blocksToInsert)
}

func TestOption(t *testing.T) {
	db := createDb(t)
	initTransferServer(t, db, 5)
	defer ts.Close()
	insertBlocks(t, db, 5)

	filter := transfers.TransferFilter{
		CriteriaSet: make([]*logdb.TransferCriteria, 0),
		Range:       nil,
		Options:     &logdb.Options{Limit: 6},
		Order:       logdb.DESC,
	}

	res, statusCode := httpPost(t, ts.URL+"/transfers", filter)
	assert.Equal(t, "options.limit exceeds the maximum allowed value of 5", strings.Trim(string(res), "\n"))
	assert.Equal(t, http.StatusForbidden, statusCode)

	filter.Options.Limit = 5
	_, statusCode = httpPost(t, ts.URL+"/transfers", filter)
	assert.Equal(t, http.StatusOK, statusCode)

	// with nil options, should use default limit, when the filtered lower
	// or equal to the limit, should return the filtered transfers
	filter.Options = nil
	res, statusCode = httpPost(t, ts.URL+"/transfers", filter)
	assert.Equal(t, http.StatusOK, statusCode)
	var tLogs []*events.FilteredEvent
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, 5, len(tLogs))

	// when the filtered transfers exceed the limit, should return the forbidden
	insertBlocks(t, db, 6)
	res, statusCode = httpPost(t, ts.URL+"/transfers", filter)
	assert.Equal(t, http.StatusForbidden, statusCode)
	assert.Equal(t, "the number of filtered logs exceeds the maximum allowed value of 5, please use pagination", strings.Trim(string(res), "\n"))
}

// Test functions
func testTransferBadRequest(t *testing.T) {
	badBody := []byte{0x00, 0x01, 0x02}

	res, err := http.Post(ts.URL+"/transfers", "application/x-www-form-urlencoded", bytes.NewReader(badBody))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func testTransferWithEmptyDb(t *testing.T) {
	emptyFilter := transfers.TransferFilter{
		CriteriaSet: make([]*logdb.TransferCriteria, 0),
		Range:       nil,
		Options:     nil,
		Order:       logdb.DESC,
	}

	res, statusCode := httpPost(t, ts.URL+"/transfers", emptyFilter)
	var tLogs []*transfers.FilteredTransfer
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, tLogs)
}

func testTransferWithBlocks(t *testing.T, expectedBlocks int) {
	emptyFilter := transfers.TransferFilter{
		CriteriaSet: make([]*logdb.TransferCriteria, 0),
		Range:       nil,
		Options:     nil,
		Order:       logdb.DESC,
	}

	res, statusCode := httpPost(t, ts.URL+"/transfers", emptyFilter)
	var tLogs []*transfers.FilteredTransfer
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, expectedBlocks, len(tLogs))
	for _, tLog := range tLogs {
		assert.NotEmpty(t, tLog)
	}
}

// Init functions
func insertBlocks(t *testing.T, db *logdb.LogDB, n int) {
	b := new(block.Builder).Build()
	for i := 0; i < n; i++ {
		b = new(block.Builder).
			ParentID(b.Header().ID()).
			Build()
		receipts := tx.Receipts{newReceipt()}

		w := db.NewWriter()
		if err := w.Write(b, receipts); err != nil {
			t.Fatal(err)
		}

		if err := w.Commit(); err != nil {
			t.Fatal(err)
		}
	}
}

func initTransferServer(t *testing.T, logDb *logdb.LogDB, limit uint64) {
	router := mux.NewRouter()

	muxDb := muxdb.NewMem()
	stater := state.NewStater(muxDb)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}

	repo, _ := chain.NewRepository(muxDb, b)

	transfers.New(repo, logDb, limit).Mount(router, "/transfers")
	ts = httptest.NewServer(router)
}

func createDb(t *testing.T) *logdb.LogDB {
	logDb, err := logdb.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	return logDb
}

// Utilities functions
func randAddress() (addr thor.Address) {
	rand.Read(addr[:])
	return
}

func newReceipt() *tx.Receipt {
	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Transfers: tx.Transfers{{
					Sender:    randAddress(),
					Recipient: randAddress(),
					Amount:    new(big.Int).SetBytes(randAddress().Bytes()),
				}},
			},
		},
	}
}

func httpPost(t *testing.T, url string, body interface{}) ([]byte, int) {
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(url, "application/x-www-form-urlencoded", bytes.NewReader(data)) // nolint: gosec
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

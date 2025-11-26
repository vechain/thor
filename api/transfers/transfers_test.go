// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transfers

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/logsdb"
	"github.com/vechain/thor/v2/logsdb/sqlite3"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

const defaultLogLimit uint64 = 1000

var (
	ts      *httptest.Server
	tclient *thorclient.Client
)

func TestEmptyTransfers(t *testing.T) {
	db := createDb(t)
	initTransferServer(t, db, defaultLogLimit)
	defer ts.Close()

	tclient = thorclient.New(ts.URL)
	testTransferBadRequest(t)
	testTransferWithEmptyDb(t)
}

func TestTransfers(t *testing.T) {
	db := createDb(t)
	initTransferServer(t, db, defaultLogLimit)
	defer ts.Close()

	tclient = thorclient.New(ts.URL)
	blocksToInsert := 5
	insertBlocks(t, db, blocksToInsert)

	testTransferWithBlocks(t, blocksToInsert)
}

func TestOption(t *testing.T) {
	db := createDb(t)
	initTransferServer(t, db, 5)
	defer ts.Close()
	insertBlocks(t, db, 5)

	tclient = thorclient.New(ts.URL)
	filter := api.TransferFilter{
		CriteriaSet: make([]*logsdb.TransferCriteria, 0),
		Range:       nil,
		Options:     &api.Options{Limit: ptr(6)},
		Order:       logsdb.DESC,
	}

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/transfer", filter)
	require.NoError(t, err)
	assert.Equal(t, "options.limit exceeds the maximum allowed value of 5", strings.Trim(string(res), "\n"))
	assert.Equal(t, http.StatusForbidden, statusCode)

	filter.Options.Limit = ptr(5)
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/logs/transfer", filter)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)

	// with nil options, should use default limit, when the filtered lower
	// or equal to the limit, should return the filtered transfers
	filter.Options = nil
	res, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/logs/transfer", filter)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	var tLogs []*api.FilteredEvent
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, 5, len(tLogs))

	// when the filtered transfers exceed the limit, should return the forbidden
	insertBlocks(t, db, 6)
	res, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/logs/transfer", filter)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, statusCode)
	assert.Equal(t, "the number of filtered logs exceeds the maximum allowed value of 5, please use pagination", strings.Trim(string(res), "\n"))
}

func TestOptionalData(t *testing.T) {
	db := createDb(t)
	initTransferServer(t, db, defaultLogLimit)
	defer ts.Close()
	insertBlocks(t, db, 5)
	tclient = thorclient.New(ts.URL)

	testCases := []struct {
		name           string
		includeIndexes bool
		expected       *uint32
	}{
		{
			name:           "do not include indexes",
			includeIndexes: false,
			expected:       nil,
		},
		{
			name:           "include indexes",
			includeIndexes: true,
			expected:       new(uint32),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter := api.TransferFilter{
				CriteriaSet: make([]*logsdb.TransferCriteria, 0),
				Range:       nil,
				Options:     &api.Options{Limit: ptr(5), IncludeIndexes: tc.includeIndexes},
				Order:       logsdb.DESC,
			}

			res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/transfer", filter)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, statusCode)
			var tLogs []*api.FilteredTransfer
			if err := json.Unmarshal(res, &tLogs); err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, http.StatusOK, statusCode)
			assert.Equal(t, 5, len(tLogs))

			for _, tLog := range tLogs {
				assert.Equal(t, tc.expected, tLog.Meta.TxIndex)
				assert.Equal(t, tc.expected, tLog.Meta.LogIndex)
			}
		})
	}
}

func TestNullCriteriaSet(t *testing.T) {
	db := createDb(t)
	initTransferServer(t, db, defaultLogLimit)
	defer ts.Close()
	tclient = thorclient.New(ts.URL)

	_, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/transfer", []byte(`{"criteriaSet": null}`))
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, statusCode)

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/transfer", []byte(`{"criteriaSet": [null]}`))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Equal(t, "criteriaSet[0]: null not allowed\n", string(res), "null criteriaSet")

	res, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/logs/transfer", []byte(`{"criteriaSet": [{}, null]}`))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Equal(t, "criteriaSet[1]: null not allowed\n", string(res), "null criteriaSet")
}

// Test functions
func testTransferBadRequest(t *testing.T) {
	badBody := []byte{0x00, 0x01, 0x02}

	_, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/transfer", badBody)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
}

func testTransferWithEmptyDb(t *testing.T) {
	emptyFilter := api.TransferFilter{
		CriteriaSet: make([]*logsdb.TransferCriteria, 0),
		Range:       nil,
		Options:     nil,
		Order:       logsdb.DESC,
	}

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/transfer", emptyFilter)
	require.NoError(t, err)
	var tLogs []*api.FilteredTransfer
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, tLogs)
}

func testTransferWithBlocks(t *testing.T, expectedBlocks int) {
	emptyFilter := api.TransferFilter{
		CriteriaSet: make([]*logsdb.TransferCriteria, 0),
		Range:       nil,
		Options:     nil,
		Order:       logsdb.DESC,
	}

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/transfer", emptyFilter)
	require.NoError(t, err)
	var tLogs []*api.FilteredTransfer
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
func insertBlocks(t *testing.T, db logsdb.LogsDB, n int) {
	b := new(block.Builder).Build()
	for range n {
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

func initTransferServer(t *testing.T, logDb logsdb.LogsDB, limit uint64) {
	thorChain, err := testchain.NewDefault()
	require.NoError(t, err)

	router := mux.NewRouter()
	New(thorChain.Repo(), logDb, limit).Mount(router, "/logs/transfer")

	ts = httptest.NewServer(router)
}

func createDb(t *testing.T) logsdb.LogsDB {
	logDb, err := sqlite3.NewMem()
	if err != nil {
		t.Fatal(err)
	}
	return logDb
}

// Utilities functions
func newReceipt() *tx.Receipt {
	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Transfers: tx.Transfers{{
					Sender:    datagen.RandAddress(),
					Recipient: datagen.RandAddress(),
					Amount:    new(big.Int).SetBytes(datagen.RandAddress().Bytes()),
				}},
			},
		},
	}
}

func ptr(v uint64) *uint64 {
	return &v
}

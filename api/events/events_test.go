// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package events_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

const defaultLogLimit uint64 = 1000

var (
	ts      *httptest.Server
	addr    = thor.BytesToAddress([]byte("address"))
	topic   = thor.BytesToBytes32([]byte("topic"))
	tclient *thorclient.Client
)

func TestEmptyEvents(t *testing.T) {
	db := createDb(t)
	initEventServer(t, db, defaultLogLimit)
	defer ts.Close()

	tclient = thorclient.New(ts.URL)
	for name, tt := range map[string]func(*testing.T){
		"testEventsBadRequest": testEventsBadRequest,
		"testEventWithEmptyDb": testEventWithEmptyDb,
	} {
		t.Run(name, tt)
	}
}

func TestEvents(t *testing.T) {
	db := createDb(t)
	initEventServer(t, db, defaultLogLimit)
	defer ts.Close()

	blocksToInsert := 5
	tclient = thorclient.New(ts.URL)
	insertBlocks(t, db, blocksToInsert)
	testEventWithBlocks(t, blocksToInsert)
}

func TestOption(t *testing.T) {
	db := createDb(t)
	initEventServer(t, db, 5)
	defer ts.Close()
	insertBlocks(t, db, 5)

	tclient = thorclient.New(ts.URL)
	filter := events.EventFilter{
		CriteriaSet: make([]*events.EventCriteria, 0),
		Range:       nil,
		Options:     &logdb.Options{Limit: 6},
		Order:       logdb.DESC,
	}

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/events", filter)
	require.NoError(t, err)
	assert.Equal(t, "options.limit exceeds the maximum allowed value of 5", strings.Trim(string(res), "\n"))
	assert.Equal(t, http.StatusForbidden, statusCode)

	filter.Options.Limit = 5
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/events", filter)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)

	// with nil options, should use default limit, when the filtered lower
	// or equal to the limit, should return the filtered events
	filter.Options = nil
	res, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/events", filter)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	var tLogs []*events.FilteredEvent
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, 5, len(tLogs))

	// when the filtered events exceed the limit, should return the forbidden
	insertBlocks(t, db, 6)
	res, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/events", filter)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, statusCode)
	assert.Equal(t, "the number of filtered logs exceeds the maximum allowed value of 5, please use pagination", strings.Trim(string(res), "\n"))
}

// Test functions
func testEventsBadRequest(t *testing.T) {
	badBody := []byte{0x00, 0x01, 0x02}

	_, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/events", badBody)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
}

func testEventWithEmptyDb(t *testing.T) {
	emptyFilter := events.EventFilter{
		CriteriaSet: make([]*events.EventCriteria, 0),
		Range:       nil,
		Options:     nil,
		Order:       logdb.DESC,
	}

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/events", emptyFilter)
	require.NoError(t, err)
	var tLogs []*events.FilteredEvent
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Empty(t, tLogs)
}

func testEventWithBlocks(t *testing.T, expectedBlocks int) {
	emptyFilter := events.EventFilter{
		CriteriaSet: make([]*events.EventCriteria, 0),
		Range:       nil,
		Options:     nil,
		Order:       logdb.DESC,
	}

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/events", emptyFilter)
	require.NoError(t, err)
	var tLogs []*events.FilteredEvent
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, expectedBlocks, len(tLogs))
	for _, tLog := range tLogs {
		assert.NotEmpty(t, tLog)
	}

	// Test with matching filter
	matchingFilter := events.EventFilter{
		CriteriaSet: []*events.EventCriteria{{
			Address: &addr,
			TopicSet: events.TopicSet{
				&topic,
				&topic,
				&topic,
				&topic,
				&topic,
			},
		}},
	}

	res, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/events", matchingFilter)
	require.NoError(t, err)
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
func initEventServer(t *testing.T, logDb *logdb.LogDB, limit uint64) {
	router := mux.NewRouter()

	muxDb := muxdb.NewMem()
	stater := state.NewStater(muxDb)
	gene := genesis.NewDevnet()

	b, _, _, err := gene.Build(stater)
	if err != nil {
		t.Fatal(err)
	}

	repo, _ := chain.NewRepository(muxDb, b)

	events.New(repo, logDb, limit).Mount(router, "/events")
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

func newReceipt() *tx.Receipt {
	return &tx.Receipt{
		Outputs: []*tx.Output{
			{
				Events: tx.Events{{
					Address: addr,
					Topics: []thor.Bytes32{
						topic,
						topic,
						topic,
						topic,
						topic,
					},
					Data: []byte("0x0"),
				}},
			},
		},
	}
}

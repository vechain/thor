// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package events_test

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
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
)

const defaultLogLimit uint64 = 1000

var (
	ts      *httptest.Server
	tclient *thorclient.Client
)

func TestEmptyEvents(t *testing.T) {
	initEventServer(t, defaultLogLimit)
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
	thorChain := initEventServer(t, defaultLogLimit)
	defer ts.Close()

	blocksToInsert := 5
	tclient = thorclient.New(ts.URL)
	insertBlocks(t, thorChain, blocksToInsert)
	testEventWithBlocks(t, blocksToInsert)
}

func TestOptionalIndexes(t *testing.T) {
	thorChain := initEventServer(t, defaultLogLimit)
	defer ts.Close()
	insertBlocks(t, thorChain, 5)
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
			filter := events.EventFilter{
				CriteriaSet: make([]*events.EventCriteria, 0),
				Range:       nil,
				Options:     &events.Options{Limit: 6, IncludeIndexes: tc.includeIndexes},
				Order:       logdb.DESC,
			}

			res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/event", filter)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, statusCode)
			var tLogs []*events.FilteredEvent
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

func TestOption(t *testing.T) {
	thorChain := initEventServer(t, 5)
	defer ts.Close()
	insertBlocks(t, thorChain, 5)

	tclient = thorclient.New(ts.URL)
	filter := events.EventFilter{
		CriteriaSet: make([]*events.EventCriteria, 0),
		Range:       nil,
		Options:     &events.Options{Limit: 6},
		Order:       logdb.DESC,
	}

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/event", filter)
	require.NoError(t, err)
	assert.Equal(t, "options.limit exceeds the maximum allowed value of 5", strings.Trim(string(res), "\n"))
	assert.Equal(t, http.StatusForbidden, statusCode)

	filter.Options.Limit = 5
	_, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/logs/event", filter)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)

	// with nil options, should use default limit, when the filtered lower
	// or equal to the limit, should return the filtered events
	filter.Options = nil
	res, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/logs/event", filter)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	var tLogs []*events.FilteredEvent
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, 5, len(tLogs))

	// when the filtered events exceed the limit, should return the forbidden
	insertBlocks(t, thorChain, 6)
	res, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/logs/event", filter)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, statusCode)
	assert.Equal(t, "the number of filtered logs exceeds the maximum allowed value of 5, please use pagination", strings.Trim(string(res), "\n"))
}

func TestZeroFrom(t *testing.T) {
	thorChain := initEventServer(t, 100)
	defer ts.Close()
	insertBlocks(t, thorChain, 5)

	tclient = thorclient.New(ts.URL)
	transferTopic := thor.MustParseBytes32("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
	criteria := []*events.EventCriteria{
		{
			TopicSet: events.TopicSet{
				Topic0: &transferTopic,
			},
		},
	}

	from := uint64(0)
	filter := events.EventFilter{
		CriteriaSet: criteria,
		Range:       &events.Range{From: &from},
		Options:     nil,
		Order:       logdb.DESC,
	}

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/event", filter)
	require.NoError(t, err)
	var tLogs []*events.FilteredEvent
	if err := json.Unmarshal(res, &tLogs); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, http.StatusOK, statusCode)
	assert.NotEmpty(t, tLogs)
}

// Test functions
func testEventsBadRequest(t *testing.T) {
	badBody := []byte{0x00, 0x01, 0x02}

	_, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/event", badBody)
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

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/event", emptyFilter)
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

	res, statusCode, err := tclient.RawHTTPClient().RawHTTPPost("/logs/event", emptyFilter)
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

	transferEvent := thor.MustParseBytes32("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

	// Test with matching filter
	matchingFilter := events.EventFilter{
		CriteriaSet: []*events.EventCriteria{{
			Address: &builtin.Energy.Address,
			TopicSet: events.TopicSet{
				Topic0: &transferEvent,
			},
		}},
	}

	res, statusCode, err = tclient.RawHTTPClient().RawHTTPPost("/logs/event", matchingFilter)
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
func initEventServer(t *testing.T, limit uint64) *testchain.Chain {
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(t, err)

	router := mux.NewRouter()
	events.New(thorChain.Repo(), thorChain.LogDB(), limit).Mount(router, "/logs/event")
	ts = httptest.NewServer(router)

	return thorChain
}

// Utilities functions
func insertBlocks(t *testing.T, chain *testchain.Chain, n int) {
	transferABI, ok := builtin.Energy.ABI.MethodByName("transfer")
	require.True(t, ok)

	encoded, err := transferABI.EncodeInput(genesis.DevAccounts()[2].Address, new(big.Int).SetUint64(datagen.RandUint64()))
	require.NoError(t, err)

	transferClause := tx.NewClause(&builtin.Energy.Address).WithData(encoded)

	for range n {
		err := chain.MintClauses(genesis.DevAccounts()[0], []*tx.Clause{transferClause})
		require.NoError(t, err)
	}
}

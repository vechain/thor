// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package restutil

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/api"
)

func requireHTTPError(t *testing.T, err error, status int, msg string) {
	t.Helper()
	var herr *httpError
	require.ErrorAs(t, err, &herr)
	assert.Equal(t, status, herr.status)
	assert.Equal(t, msg, herr.Error())
}

func TestPrepareLogFilter(t *testing.T) {
	const maxLimit, maxOffset, maxCriteria = 10, 100, 2
	u64 := func(v uint64) *uint64 { return &v }
	crit := func() *api.TransferCriteria { return &api.TransferCriteria{} }

	t.Run("defaults nil options to limit+1", func(t *testing.T) {
		opts, err := PrepareLogFilter[api.TransferCriteria](nil, nil, nil, maxLimit, maxOffset, maxCriteria)
		require.NoError(t, err)
		assert.Equal(t, &api.Options{Limit: u64(maxLimit + 1)}, opts)
	})

	t.Run("keeps given options and fills a missing limit", func(t *testing.T) {
		in := &api.Options{Offset: 5, IncludeIndexes: true}
		opts, err := PrepareLogFilter[api.TransferCriteria](in, nil, nil, maxLimit, maxOffset, maxCriteria)
		require.NoError(t, err)
		assert.Same(t, in, opts)
		assert.Equal(t, &api.Options{Offset: 5, Limit: u64(maxLimit + 1), IncludeIndexes: true}, opts)

		in = &api.Options{Limit: u64(3)}
		opts, err = PrepareLogFilter[api.TransferCriteria](in, nil, nil, maxLimit, maxOffset, maxCriteria)
		require.NoError(t, err)
		assert.Equal(t, u64(3), opts.Limit)
	})

	t.Run("limit over max is 403", func(t *testing.T) {
		_, err := PrepareLogFilter[api.TransferCriteria](&api.Options{Limit: u64(maxLimit + 1)}, nil, nil, maxLimit, maxOffset, maxCriteria)
		requireHTTPError(t, err, http.StatusForbidden, "options.limit exceeds the maximum allowed value of 10")
	})

	t.Run("offset over max is 403", func(t *testing.T) {
		_, err := PrepareLogFilter[api.TransferCriteria](&api.Options{Offset: maxOffset + 1}, nil, nil, maxLimit, maxOffset, maxCriteria)
		requireHTTPError(t, err, http.StatusForbidden, "options.offset exceeds the maximum allowed value of 100")
	})

	t.Run("invalid range is 400", func(t *testing.T) {
		_, err := PrepareLogFilter[api.TransferCriteria](nil, &api.Range{Unit: "days"}, nil, maxLimit, maxOffset, maxCriteria)
		requireHTTPError(t, err, http.StatusBadRequest, "filter.Range.Unit must be either 'block' or 'time', got 'days'")

		_, err = PrepareLogFilter[api.TransferCriteria](nil, &api.Range{From: u64(5), To: u64(1)}, nil, maxLimit, maxOffset, maxCriteria)
		requireHTTPError(t, err, http.StatusBadRequest, "filter.Range.To must be greater than or equal to filter.Range.From")
	})

	t.Run("null criterion is 400", func(t *testing.T) {
		_, err := PrepareLogFilter(nil, nil, []*api.TransferCriteria{crit(), nil}, maxLimit, maxOffset, maxCriteria)
		requireHTTPError(t, err, http.StatusBadRequest, "criteriaSet[1]: null not allowed")
	})

	t.Run("too many criteria is 400", func(t *testing.T) {
		_, err := PrepareLogFilter(nil, nil, []*api.TransferCriteria{crit(), crit(), crit()}, maxLimit, maxOffset, maxCriteria)
		requireHTTPError(t, err, http.StatusBadRequest, "number of criteria in criteriaSet: 3 cannot be greater than: 2")
	})

	t.Run("checks run in order", func(t *testing.T) {
		// options are checked before the range, the range before the criteria
		_, err := PrepareLogFilter(
			&api.Options{Limit: u64(maxLimit + 1)},
			&api.Range{Unit: "days"},
			[]*api.TransferCriteria{nil},
			maxLimit,
			maxOffset,
			maxCriteria,
		)
		requireHTTPError(t, err, http.StatusForbidden, "options.limit exceeds the maximum allowed value of 10")

		_, err = PrepareLogFilter(nil, &api.Range{Unit: "days"}, []*api.TransferCriteria{nil}, maxLimit, maxOffset, maxCriteria)
		requireHTTPError(t, err, http.StatusBadRequest, "filter.Range.Unit must be either 'block' or 'time', got 'days'")

		// a null criterion is reported before the count
		_, err = PrepareLogFilter(nil, nil, []*api.TransferCriteria{nil, crit(), crit()}, maxLimit, maxOffset, maxCriteria)
		requireHTTPError(t, err, http.StatusBadRequest, "criteriaSet[0]: null not allowed")
	})
}

func TestCheckLogCount(t *testing.T) {
	require.NoError(t, CheckLogCount(0, 10))
	require.NoError(t, CheckLogCount(10, 10))
	requireHTTPError(t, CheckLogCount(11, 10), http.StatusForbidden,
		"the number of filtered logs exceeds the maximum allowed value of 10, please use pagination")
}

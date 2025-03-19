// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"strings"

	"github.com/vechain/thor/v2/metrics"
)

var (
	metricCriteriaLengthBucket = metrics.LazyLoadHistogramVec("logdb_criteria_length_bucket", []string{"type"}, []int64{0, 2, 5, 10, 25, 100, 1000})
	metricEventQueryParameters = metrics.LazyLoadCounterVec("logdb_query_parameters", []string{"parameters"})
	metricQueryOrderCounter    = metrics.LazyLoadCounterVec("logdb_query_order", []string{"order"})
	metricOffsetBucket         = metrics.LazyLoadHistogramVec("logdb_query_offset_bucket", []string{"type"}, []int64{
		0, 1_000, 5_000, 10_000, 25_000, 50_000, 100_000, 250_000, 500_000, 1_000_000,
	})
	metricLimitBucket = metrics.LazyLoadHistogramVec("logdb_query_limit_bucket", []string{"type"}, []int64{
		0, 5, 10, 25, 50, 100, 250, 500, 1000,
	})
)

func metricsHandleEventsFilter(filter *EventFilter) {
	if metrics.NoOp() {
		return
	}

	metricsHandleCommon(filter.Options, filter.Order, len(filter.CriteriaSet), "event")

	for _, c := range filter.CriteriaSet {
		paramsUsed := make([]string, 0)
		if c.Address != nil {
			paramsUsed = append(paramsUsed, "address")
		}
		if c.Topics[0] != nil {
			paramsUsed = append(paramsUsed, "topic0")
		}
		if c.Topics[1] != nil {
			paramsUsed = append(paramsUsed, "topic1")
		}
		if c.Topics[2] != nil {
			paramsUsed = append(paramsUsed, "topic2")
		}
		if c.Topics[3] != nil {
			paramsUsed = append(paramsUsed, "topic3")
		}
		if c.Topics[4] != nil {
			paramsUsed = append(paramsUsed, "topic4")
		}
		metricEventQueryParameters().AddWithLabel(1, map[string]string{"parameters": strings.Join(paramsUsed, ",")})
	}
}

func metricsHandleCommon(options *Options, order Order, criteriaLen int, queryType string) {
	if metrics.NoOp() {
		return
	}

	metricCriteriaLengthBucket().ObserveWithLabels(int64(criteriaLen), map[string]string{"type": "transfer"})

	if order == DESC {
		metricQueryOrderCounter().AddWithLabel(1, map[string]string{"order": "desc", "type": queryType})
	} else {
		metricQueryOrderCounter().AddWithLabel(1, map[string]string{"order": "asc", "type": queryType})
	}

	offset := options.Offset
	if offset > 1_000_000 {
		offset = 1_000_001
	}
	metricOffsetBucket().ObserveWithLabels(int64(offset), map[string]string{"type": queryType})

	limit := options.Limit
	if limit > 1000 {
		limit = 1001
	}
	metricLimitBucket().ObserveWithLabels(int64(limit), map[string]string{"type": queryType})
}

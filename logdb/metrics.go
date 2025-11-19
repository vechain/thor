// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"fmt"
	"strings"

	"github.com/vechain/thor/v2/metrics"
)

var (
	metricCriteriaLengthBucket        = metrics.LazyLoadHistogramVec("logdb_criteria_length_bucket", []string{"type"}, []int64{0, 2, 5, 10, 25, 100, 1000})
	metricEventQueryParametersCounter = metrics.LazyLoadCounterVec("logdb_query_parameters_counter", []string{"parameters"})
	metricQueryOrderCounter           = metrics.LazyLoadCounterVec("logdb_query_order_counter", []string{"order", "type"})
	metricOffsetBucket                = metrics.LazyLoadHistogramVec("logdb_query_offset_bucket", []string{"type"}, []int64{
		0, 1_000, 5_000, 10_000, 25_000, 50_000, 100_000,
	})
	metricLimitBucket = metrics.LazyLoadHistogramVec("logdb_query_limit_bucket", []string{"type"}, []int64{
		0, 5, 10, 25, 50, 100, 250, 500, 1000,
	})
)

func MetricsHandleEventsFilter(filter *EventFilter) {
	MetricsHandleCommonFilter(filter.Options, filter.Order, len(filter.CriteriaSet), "event")

	for _, c := range filter.CriteriaSet {
		paramsUsed := make([]string, 0)
		if c.Address != nil {
			paramsUsed = append(paramsUsed, "address")
		}
		for i, t := range c.Topics {
			if t != nil {
				paramsUsed = append(paramsUsed, fmt.Sprintf("topic%d", i))
			}
		}
		metricEventQueryParametersCounter().AddWithLabel(1, map[string]string{"parameters": strings.Join(paramsUsed, ",")})
	}
}

func MetricsHandleCommonFilter(options *Options, order Order, criteriaLen int, queryType string) {
	orderStr := "asc"
	if order == DESC {
		orderStr = "desc"
	}

	if options == nil {
		options = &Options{}
	}

	offset := options.Offset
	if offset > 1_000_000 {
		offset = 1_000_001
	}

	limit := options.Limit
	if limit > 1000 {
		limit = 1001
	}

	metricCriteriaLengthBucket().ObserveWithLabels(int64(criteriaLen), map[string]string{"type": queryType})
	metricOffsetBucket().ObserveWithLabels(int64(offset), map[string]string{"type": queryType})
	metricLimitBucket().ObserveWithLabels(int64(limit), map[string]string{"type": queryType})
	metricQueryOrderCounter().AddWithLabel(1, map[string]string{"order": orderStr, "type": queryType})
}

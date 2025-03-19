package logdb

import (
	"strings"

	"github.com/vechain/thor/v2/metrics"
)

var (
	metricCriteriaLengthBucket   = metrics.LazyLoadHistogramVec("logdb_criteria_length_bucket", []string{"type"}, []int64{0, 2, 5, 10, 25, 100, 1000})
	metricEventQueryTypes        = metrics.LazyLoadCounterVec("logdb_query_types", []string{"type"})
	metricEventQueryOrderCounter = metrics.LazyLoadCounterVec("logdb_query_order", []string{"order"})
	metricEventOffsetBucket      = metrics.LazyLoadHistogramVec("logdb_query_offset_bucket", []string{"type"}, []int64{
		0, 1_000, 5_000, 10_000, 25_000, 50_000, 100_000, 250_000, 500_000, 1_000_000,
	})
	metricEventLimitBucket = metrics.LazyLoadHistogramVec("logdb_query_limit_bucket", []string{"type"}, []int64{
		0, 5, 10, 25, 50, 100, 250, 500, 1000,
	})
)

func metricsHandleEventsFilter(filter *EventFilter) {
	if metrics.NoOp() {
		return
	}

	metricsHandleCommon(filter.Options, filter.Order, len(filter.CriteriaSet), "event")

	for _, c := range filter.CriteriaSet {
		queryTypes := make([]string, 0)
		if c.Address != nil {
			queryTypes = append(queryTypes, "address")
		}
		if c.Topics[0] != nil {
			queryTypes = append(queryTypes, "topic0")
		}
		if c.Topics[1] != nil {
			queryTypes = append(queryTypes, "topic1")
		}
		if c.Topics[2] != nil {
			queryTypes = append(queryTypes, "topic2")
		}
		if c.Topics[3] != nil {
			queryTypes = append(queryTypes, "topic3")
		}
		if c.Topics[4] != nil {
			queryTypes = append(queryTypes, "topic4")
		}
		metricEventQueryTypes().AddWithLabel(1, map[string]string{"type": strings.Join(queryTypes, ",")})
	}
}

func metricsHandleCommon(options *Options, order Order, criteriaLen int, queryType string) {
	if metrics.NoOp() {
		return
	}

	metricCriteriaLengthBucket().ObserveWithLabels(int64(criteriaLen), map[string]string{"type": "transfer"})

	if order == DESC {
		metricEventQueryOrderCounter().AddWithLabel(1, map[string]string{"order": "desc", "type": queryType})
	} else {
		metricEventQueryOrderCounter().AddWithLabel(1, map[string]string{"order": "asc", "type": queryType})
	}

	offset := options.Offset
	if offset > 1_000_000 {
		offset = 1_000_001
	}
	metricEventOffsetBucket().ObserveWithLabels(int64(offset), map[string]string{"type": queryType})

	limit := options.Limit
	if limit > 1000 {
		limit = 1001
	}
	metricEventLimitBucket().ObserveWithLabels(int64(limit), map[string]string{"type": queryType})
}

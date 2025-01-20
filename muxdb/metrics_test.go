package muxdb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricsCollectCompactionValues(t *testing.T) {
	require.NotPanics(t, func() { collectCompactionValues("") })
	require.NotPanics(t, func() { collectCompactionValues("wrong stats") })

	stats := ` Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)
-------+------------+---------------+---------------+---------------+---------------
   0   |          0 |       0.00000 |       0.16840 |       0.00000 |      61.67909
   1   |         27 |      96.34199 |       1.03280 |     139.39040 |     138.68919
   2   |        271 |     989.34527 |       0.15046 |      45.49008 |      39.92714
   3   |       2732 |   10002.10112 |       1.11660 |     128.58780 |     119.32566
   4   |       3544 |   13591.24199 |       3.38804 |    2059.54114 |     223.60823
-------+------------+---------------+---------------+---------------+---------------
 Total |       6574 |   24679.03037 |       5.85630 |    2373.00942 |     583.22930
`
	values, err := extractCompactionValues(stats)

	require.Equal(t, nil, err)
	require.Equal(t, "0", values[0].Level)
	require.Equal(t, int64(27), values[1].Tables)
	require.Equal(t, int64(989), values[2].SizeMB)
}

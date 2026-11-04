// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package httpserver

import (
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/gorilla/handlers"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/middleware"
	"github.com/vechain/thor/v2/api/restutil"
)

type benchItem struct {
	ID   int    `json:"id"`
	Data string `json:"data"`
}

const (
	benchItems    = 100_000
	benchItemData = "0123456789abcdef0123456789abcdef0123456789abcdef"
)

// newBenchServer starts a real TCP server with the write deadline applied and returns its URL.
func newBenchServer(b *testing.B, h http.Handler) string {
	b.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	srv := &http.Server{Handler: middleware.HandleWriteDeadline(api.DefaultWriteTimeout)(h), ReadHeaderTimeout: api.DefaultWriteTimeout}
	go srv.Serve(l)
	b.Cleanup(func() { srv.Close() })
	return "http://" + l.Addr().String()
}

func BenchmarkWriteDeadline(b *testing.B) {
	array := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = restutil.WriteJSONArray(w, benchItems, func(i int) benchItem {
			return benchItem{ID: i, Data: benchItemData}
		})
	})
	single := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		items := make([]benchItem, benchItems)
		for i := range items {
			items[i] = benchItem{ID: i, Data: benchItemData}
		}
		_ = restutil.WriteJSON(w, items)
	})

	// uncompressed size, used for SetBytes
	var size int64
	{
		url := newBenchServer(b, array)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			b.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		size, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	cases := []struct {
		name string
		h    http.Handler
		gzip bool
	}{
		{"array-gzip", array, true},
		{"array-plain", array, false},
		{"single-plain", single, false},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			url := newBenchServer(b, handlers.CompressHandler(c.h))
			client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
			b.Cleanup(client.CloseIdleConnections)
			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req, _ := http.NewRequest(http.MethodGet, url, nil)
				if c.gzip {
					req.Header.Set("Accept-Encoding", "gzip")
				}
				resp, err := client.Do(req)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := io.Copy(io.Discard, resp.Body); err != nil {
					b.Fatal(err)
				}
				resp.Body.Close()
			}
		})
	}
}

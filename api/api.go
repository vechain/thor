// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/debug"
	"github.com/vechain/thor/v2/api/doc"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/api/subscriptions"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/api/transfers"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

var logger = log.WithContext("pkg", "api")

type Options struct {
	AllowedOrigins    string
	BacktraceLimit    uint32
	CallGasLimit      uint64
	PprofOn           bool
	SkipLogs          bool
	AllowCustomTracer bool
	EnableReqLogger   bool
	EnableMetrics     bool
	LogsLimit         uint64
	AllowedTracers    map[string]interface{}
	SoloMode          bool
}

// New return api router
func New(
	repo *chain.Repository,
	stater *state.Stater,
	txPool *txpool.TxPool,
	logDB *logdb.LogDB,
	bft bft.Finalizer,
	nw node.Network,
	forkConfig thor.ForkConfig,
	opts Options, // Changed to accept Options struct
) (http.HandlerFunc, func()) {
	origins := strings.Split(strings.TrimSpace(opts.AllowedOrigins), ",")
	for i, o := range origins {
		origins[i] = strings.ToLower(strings.TrimSpace(o))
	}

	router := mux.NewRouter()

	// to serve stoplight, swagger and api docs
	router.PathPrefix("/doc").Handler(
		http.StripPrefix("/doc/", http.FileServer(http.FS(doc.FS))),
	)

	// redirect stoplight-ui
	router.Path("/").HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "doc/stoplight-ui/", http.StatusTemporaryRedirect)
		})

	accounts.New(repo, stater, opts.CallGasLimit, forkConfig, bft).
		Mount(router, "/accounts")

	if !opts.SkipLogs {
		events.New(repo, logDB, opts.LogsLimit).
			Mount(router, "/logs/event")
		transfers.New(repo, logDB, opts.LogsLimit).
			Mount(router, "/logs/transfer")
	}
	blocks.New(repo, bft).
		Mount(router, "/blocks")
	transactions.New(repo, txPool).
		Mount(router, "/transactions")
	debug.New(repo, stater, forkConfig, opts.CallGasLimit, opts.AllowCustomTracer, bft, opts.AllowedTracers, opts.SoloMode).
		Mount(router, "/debug")
	node.New(nw).
		Mount(router, "/node")
	subs := subscriptions.New(repo, origins, opts.BacktraceLimit, txPool)
	subs.Mount(router, "/subscriptions")

	if opts.PprofOn {
		router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		router.HandleFunc("/debug/pprof/trace", pprof.Trace)
		router.PathPrefix("/debug/pprof/").HandlerFunc(pprof.Index)
	}

	if opts.EnableMetrics {
		router.Use(metricsMiddleware)
	}

	handler := handlers.CompressHandler(router)
	handler = handlers.CORS(
		handlers.AllowedOrigins(origins),
		handlers.AllowedHeaders([]string{"content-type", "x-genesis-id"}),
		handlers.ExposedHeaders([]string{"x-genesis-id", "x-thorest-ver"}),
	)(handler)

	if opts.EnableReqLogger {
		handler = RequestLoggerHandler(handler, logger)
	}

	return handler.ServeHTTP, subs.Close // subscriptions handles hijacked conns, which need to be closed
}

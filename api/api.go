// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"net/http"
	"net/http/pprof"
	"strings"
	"sync/atomic"

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

type Config struct {
	AllowedOrigins    string
	BacktraceLimit    uint32
	CallGasLimit      uint64
	PprofOn           bool
	SkipLogs          bool
	AllowCustomTracer bool
	EnableReqLogger   *atomic.Bool
	EnableMetrics     bool
	LogsLimit         uint64
	AllowedTracers    []string
	SoloMode          bool
	EnableDeprecated  bool
	EnableMempool     bool
}

// New return api router
func New(
	repo *chain.Repository,
	stater *state.Stater,
	txPool *txpool.TxPool,
	logDB *logdb.LogDB,
	bft bft.Committer,
	nw node.Network,
	forkConfig thor.ForkConfig,
	config Config,
) (http.HandlerFunc, func()) {
	origins := strings.Split(strings.TrimSpace(config.AllowedOrigins), ",")
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

	accounts.New(repo, stater, config.CallGasLimit, forkConfig, bft, config.EnableDeprecated).
		Mount(router, "/accounts")

	if !config.SkipLogs {
		events.New(repo, logDB, config.LogsLimit).
			Mount(router, "/logs/event")
		transfers.New(repo, logDB, config.LogsLimit).
			Mount(router, "/logs/transfer")
	}
	blocks.New(repo, bft).
		Mount(router, "/blocks")
	transactions.New(repo, txPool).
		Mount(router, "/transactions")
	debug.New(repo, stater, forkConfig, config.CallGasLimit, config.AllowCustomTracer, bft, config.AllowedTracers, config.SoloMode).
		Mount(router, "/debug")
	node.New(nw, txPool).
		Mount(router, "/node", config.EnableMempool)
	subs := subscriptions.New(repo, origins, config.BacktraceLimit, txPool, config.EnableDeprecated)
	subs.Mount(router, "/subscriptions")

	if config.PprofOn {
		router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		router.HandleFunc("/debug/pprof/trace", pprof.Trace)
		router.PathPrefix("/debug/pprof/").HandlerFunc(pprof.Index)
	}

	if config.EnableMetrics {
		router.Use(metricsMiddleware)
	}

	handler := handlers.CompressHandler(router)
	handler = handlers.CORS(
		handlers.AllowedOrigins(origins),
		handlers.AllowedHeaders([]string{"content-type", "x-genesis-id"}),
		handlers.ExposedHeaders([]string{"x-genesis-id", "x-thorest-ver"}),
	)(handler)

	handler = RequestLoggerHandler(handler, logger, config.EnableReqLogger)

	return handler.ServeHTTP, subs.Close // subscriptions handles hijacked conns, which need to be closed
}

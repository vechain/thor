// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package httpserver

import (
	"net"
	"net/http"
	"net/http/pprof"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/debug"
	"github.com/vechain/thor/v2/api/doc"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/api/fees"
	"github.com/vechain/thor/v2/api/middleware"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/api/subscriptions"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/api/transfers"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

var logger = log.WithContext("pkg", "api")

const (
	defaultFeeCacheSize     = 1024
	defaultRequestBodyLimit = 200 * 1024 // 200KB
)

type APIConfig struct {
	AllowedOrigins             string
	BacktraceLimit             uint32
	CallGasLimit               uint64
	PprofOn                    bool
	SkipLogs                   bool
	AllowCustomTracer          bool
	EnableReqLogger            *atomic.Bool
	EnableMetrics              bool
	LogsLimit                  uint64
	AllowedTracers             []string
	SoloMode                   bool
	EnableDeprecated           bool
	EnableTxPool               bool
	APIBacktraceLimit          int
	PriorityIncreasePercentage int
	Timeout                    int
}

func StartAPIServer(
	addr string,
	repo *chain.Repository,
	stater *state.Stater,
	txPool txpool.Pool,
	logDB *logdb.LogDB,
	bft bft.Committer,
	nw api.Network,
	forkConfig *thor.ForkConfig,
	config APIConfig,
) (string, func(), error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, errors.Wrapf(err, "listen API addr [%v]", addr)
	}

	origins := strings.Split(strings.TrimSpace(config.AllowedOrigins), ",")
	for i, o := range origins {
		origins[i] = strings.ToLower(strings.TrimSpace(o))
	}

	router := mux.NewRouter()

	// to serve stoplight, swagger and api docs
	router.Path("/doc/thor.yaml").HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/x-yaml")
			w.Write(doc.Thoryaml)
		})

	router.PathPrefix("/doc").Handler(
		http.StripPrefix("/doc/", http.FileServer(http.FS(doc.FS))),
	)

	// redirect stoplight-ui
	router.Path("/").HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "doc/stoplight-ui/", http.StatusTemporaryRedirect)
		})

	accounts.New(repo, stater, config.CallGasLimit, forkConfig, bft, config.EnableDeprecated).Mount(router, "/accounts")
	if !config.SkipLogs {
		events.New(repo, logDB, config.LogsLimit).Mount(router, "/logs/event")
		transfers.New(repo, logDB, config.LogsLimit).Mount(router, "/logs/transfer")
	}
	blocks.New(repo, bft).Mount(router, "/blocks")
	transactions.New(repo, txPool).Mount(router, "/transactions")
	debug.New(repo, stater, forkConfig, bft,
		config.CallGasLimit,
		config.AllowCustomTracer,
		config.AllowedTracers,
		config.SoloMode,
	).Mount(router, "/debug")
	node.New(nw, txPool, config.EnableTxPool).Mount(router, "/node")
	fees.New(repo, bft, forkConfig, stater, fees.Config{
		APIBacktraceLimit:          config.APIBacktraceLimit,
		PriorityIncreasePercentage: config.PriorityIncreasePercentage,
		FixedCacheSize:             defaultFeeCacheSize,
	}).Mount(router, "/fees")
	subs := subscriptions.New(repo, origins, config.BacktraceLimit, txPool, config.EnableDeprecated)
	subs.Mount(router, "/subscriptions")

	if config.PprofOn {
		router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		router.HandleFunc("/debug/pprof/trace", pprof.Trace)
		router.PathPrefix("/debug/pprof/").HandlerFunc(pprof.Index)
	}

	// middlewares
	// body limit and timeout
	router.Use(middleware.HandleRequestBodyLimit(defaultRequestBodyLimit))
	if config.Timeout > 0 {
		router.Use(middleware.HandleAPITimeout(time.Duration(config.Timeout) * time.Millisecond))
	}

	// metrics and request logger should be configured as soon as possible
	router.Use(middleware.RequestLoggerMiddleware(logger, config.EnableReqLogger))
	if config.EnableMetrics {
		router.Use(middleware.MetricsMiddleware)
	}

	router.Use(middleware.HandleXGenesisID(repo.GenesisBlock().Header().ID()))
	router.Use(middleware.HandleXThorestVersion)

	router.Use(handlers.CompressHandler)
	handler := handlers.CORS(
		handlers.AllowedOrigins(origins),
		handlers.AllowedHeaders([]string{"content-type", "x-genesis-id"}),
		handlers.ExposedHeaders([]string{"x-genesis-id", "x-thorest-ver"}),
	)(router)
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second}
	var goes co.Goes
	goes.Go(func() {
		srv.Serve(listener)
	})
	return "http://" + listener.Addr().String() + "/", func() {
		srv.Close()
		subs.Close()
		goes.Wait()
	}, nil
}

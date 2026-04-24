// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rpc

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"time"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

// Server is the POST /rpc handler. It owns the dispatch table and the
// business-layer dependencies every handler shares; handlers are
// stateless functions that receive a *Server plus the raw params.
type Server struct {
	repo       *chain.Repository
	stater     *state.Stater
	pool       txpool.Pool
	logDB      *logdb.LogDB
	forkConfig *thor.ForkConfig
	bft        bft.Committer
	cfg        Config

	// dispatch is populated from the package-level method registry once per
	// Server; it's a value copy so in-test registration doesn't leak across
	// servers (tests can call the exported RegisterHandler on a fresh
	// instance without touching the global map).
	dispatch map[string]handlerFunc
}

// handlerFunc is the common shape every eth_* method implements. Returning a
// non-nil *RPCError overrides any non-nil result and emits an error envelope.
type handlerFunc func(ctx context.Context, s *Server, params json.RawMessage) (any, *RPCError)

// NewServer constructs a Server bound to Thor's data sources. The dispatch
// table is snapshotted from the package-level map populated by init()
// functions in the per-handler files; see handler_chain.go etc.
func NewServer(
	repo *chain.Repository,
	stater *state.Stater,
	pool txpool.Pool,
	logDB *logdb.LogDB,
	forkConfig *thor.ForkConfig,
	bft bft.Committer,
	cfg Config,
) *Server {
	s := &Server{
		repo:       repo,
		stater:     stater,
		pool:       pool,
		logDB:      logDB,
		forkConfig: forkConfig,
		bft:        bft,
		cfg:        cfg,
		dispatch:   make(map[string]handlerFunc, len(globalHandlers)),
	}
	maps.Copy(s.dispatch, globalHandlers)
	return s
}

// --- Global dispatch registry -------------------------------------------------

// globalHandlers is populated by init() in each handler file. This keeps each
// file self-registering and avoids a mega switch / central registry file.
var globalHandlers = map[string]handlerFunc{}

// register installs a handler under the given JSON-RPC method name. Called
// from init() in the individual handler_*.go files. Panics on duplicate
// registration — duplicate method names are a programming error, not a
// runtime condition.
func register(method string, fn handlerFunc) {
	if _, dup := globalHandlers[method]; dup {
		panic("rpc: duplicate handler registration for " + method)
	}
	globalHandlers[method] = fn
}

// ServeHTTP implements http.Handler. Every request, success or failure, emits
// exactly one JSON-RPC 2.0 response envelope at HTTP status 200. Only the
// transport-level errors (method != POST, body read) emit non-200 statuses.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.bodyLimit()))
	defer r.Body.Close()
	if err != nil {
		// MaxBytesReader produces http.MaxBytesError after the limit; treat
		// either error as a parse-time failure rather than a transport one so
		// the client gets a JSON-RPC envelope back.
		writeEnvelope(w, rpcResponse{JSONRPC: "2.0", Error: ReasonError(ReasonOversizedData, "request body too large: "+err.Error())})
		return
	}

	env := s.dispatchAndLog(r.Context(), body, start)
	writeEnvelope(w, env)
}

// dispatchAndLog handles the dispatch logic for a single JSON-RPC request,
// returning the response envelope. The ctx and start parameters are reserved
// for Task 1.3's logging middleware (a future defer will emit one log line per
// call using these values). The named return value env allows that future defer
// to read the final envelope without additional wiring.
func (s *Server) dispatchAndLog(ctx context.Context, body []byte, start time.Time) (env rpcResponse) {
	_ = start // reserved for Task 1.3 logging

	// Reject array-form (batch) requests up-front with a clear message.
	// Scan for the first non-whitespace byte.
	if firstNonSpace(body) == '[' {
		env = rpcResponse{JSONRPC: "2.0", Error: InvalidRequest("batch requests not supported")}
		return
	}

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		env = rpcResponse{JSONRPC: "2.0", Error: ParseError(err.Error())}
		return
	}

	// Validate envelope.
	if req.JSONRPC != "2.0" {
		env = rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: InvalidRequest("jsonrpc version must be \"2.0\"")}
		return
	}
	if req.Method == "" {
		env = rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: InvalidRequest("method required")}
		return
	}

	handler, ok := s.dispatch[req.Method]
	if !ok {
		env = rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: MethodNotFound(req.Method)}
		return
	}

	result, rpcErr := handler(ctx, s, req.Params)
	if rpcErr != nil {
		env = rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}
		return
	}
	env = rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result, resultSet: true}
	return
}

// writeEnvelope serializes the response envelope and writes it to w. On
// marshal failure (which should only happen if a handler returns a
// non-marshalable result) a -32603 envelope is synthesized as a fallback.
func writeEnvelope(w http.ResponseWriter, env rpcResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	raw, err := json.Marshal(env)
	if err != nil {
		fallback, _ := json.Marshal(rpcResponse{JSONRPC: "2.0", ID: env.ID, Error: InternalError(err)})
		w.WriteHeader(http.StatusOK)
		w.Write(fallback)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(raw)
}

func firstNonSpace(b []byte) byte {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return c
		}
	}
	return 0
}


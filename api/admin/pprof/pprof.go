// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

// Package pprof wires net/http/pprof handlers under /debug/pprof/* on a
// gorilla/mux router root, gated by a featuregate.Gate so they only serve
// while pprof is enabled at runtime.
package pprof

import (
	"net/http/pprof"

	"github.com/gorilla/mux"

	"github.com/vechain/thor/v2/api/admin/featuregate"
)

// MountHandlers registers the pprof endpoints under /debug/pprof/* on root.
// They are protected by gate.Middleware and return 503 while disabled.
// The prefix is fixed because net/http/pprof.Index hard-codes it.
func MountHandlers(root *mux.Router, gate *featuregate.Gate) {
	sub := root.PathPrefix("/debug/pprof").Subrouter()
	sub.Use(gate.Middleware)
	sub.HandleFunc("/cmdline", pprof.Cmdline)
	sub.HandleFunc("/profile", pprof.Profile)
	sub.HandleFunc("/symbol", pprof.Symbol)
	sub.HandleFunc("/trace", pprof.Trace)
	sub.PathPrefix("/").HandlerFunc(pprof.Index)
}

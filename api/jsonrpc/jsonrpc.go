// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/vechain/thor/v2/api/jsonrpc/server"
	"github.com/vechain/thor/v2/api/jsonrpc/service"
	"github.com/vechain/thor/v2/chain"
)

// JSONRPC composes the JSON-RPC engine (server) with the thor business services.
type JSONRPC struct {
	server *server.Server
}

// New creates a JSONRPC instance with the eth and net namespaces registered.
func New(repo *chain.Repository) *JSONRPC {
	srv := server.New()
	b := service.NewBackend(repo)

	for _, reg := range []struct {
		namespace string
		service   any
	}{
		{"eth", service.NewEth(b)},
		{"net", service.NewNet(b)},
	} {
		if err := srv.RegisterName(reg.namespace, reg.service); err != nil {
			panic(fmt.Sprintf("jsonrpc: register namespace %q: %v", reg.namespace, err))
		}
	}

	return &JSONRPC{server: srv}
}

// Mount attaches the JSON-RPC POST endpoint under pathPrefix on root.
func (j *JSONRPC) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("").Methods(http.MethodPost).Name("POST /rpc").Handler(j.server)
	sub.Path("/").Methods(http.MethodPost).Handler(j.server) // tolerate trailing slash
}

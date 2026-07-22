// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package jsonrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type JSONRPC struct {
	server *Server
}

func New(repo *chain.Repository, stater *state.Stater, bft bft.Committer, forkConfig *thor.ForkConfig) *JSONRPC {
	srv := NewServer()
	b := &backend{repo: repo, stater: stater, bft: bft, forkConfig: forkConfig}

	for _, reg := range []struct {
		namespace string
		service   interface{}
	}{
		{"eth", &ethAPI{b: b}},
		{"net", &netAPI{b: b}},
		{"web3", &web3API{}},
	} {
		if err := srv.RegisterName(reg.namespace, reg.service); err != nil {
			panic(fmt.Sprintf("jsonrpc: register namespace %q: %v", reg.namespace, err))
		}
	}

	return &JSONRPC{server: srv}
}

func (j *JSONRPC) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("").Methods(http.MethodPost).Name("POST /rpc").
		HandlerFunc(restutil.WrapHandlerFunc(j.handleHTTP))
}

func (j *JSONRPC) handleHTTP(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return restutil.WriteJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: err.Error()}))
	}
	ctx := r.Context()
	trimmed := bytes.TrimLeft(body, " \t\r\n")

	if len(trimmed) > 0 && trimmed[0] == '[' {
		var msgs []jsonrpcMessage
		if err := json.Unmarshal(body, &msgs); err != nil || len(msgs) == 0 {
			return restutil.WriteJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: "invalid batch"}))
		}
		resps := make([]*jsonrpcMessage, len(msgs))
		for i := range msgs {
			resps[i] = j.server.handleMsg(ctx, &msgs[i])
		}
		return restutil.WriteJSON(w, resps)
	}

	var msg jsonrpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return restutil.WriteJSON(w, errorResponse(nil, &jsonError{Code: errcodeParse, Message: err.Error()}))
	}
	return restutil.WriteJSON(w, j.server.handleMsg(ctx, &msg))
}

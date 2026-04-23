// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package httpserver

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/handlers"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api/ethcompat"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

// StartEthRPCServer starts the Ethereum JSON-RPC 2.0 compatibility server.
func StartEthRPCServer(
	addr string,
	allowedOrigins string,
	repo *chain.Repository,
	stater *state.Stater,
	txPool txpool.Pool,
	logDB *logdb.LogDB,
	bft bft.Committer,
	forkConfig *thor.ForkConfig,
	callGasLimit uint64,
	version string,
) (string, func(), error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, errors.Wrapf(err, "listen ETH RPC addr [%v]", addr)
	}

	origins := strings.Split(strings.TrimSpace(allowedOrigins), ",")
	for i, o := range origins {
		origins[i] = strings.TrimSpace(o)
	}

	rpc := ethcompat.New(repo, stater, txPool, logDB, bft, forkConfig, callGasLimit, version)
	srv := &http.Server{
		Handler:           handlers.CORS(handlers.AllowedOrigins(origins))(rpc),
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       5 * time.Second,
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Serve(listener)
	}()
	return "http://" + listener.Addr().String(), func() {
		srv.Close()
		wg.Wait()
	}, nil
}

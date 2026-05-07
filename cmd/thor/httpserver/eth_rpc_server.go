// Copyright (c) 2026 The VeChainThor developers
//
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

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/rpc"
	"github.com/vechain/thor/v2/rpc/accounts"
	"github.com/vechain/thor/v2/rpc/blocks"
	rpcchain "github.com/vechain/thor/v2/rpc/chain"
	"github.com/vechain/thor/v2/rpc/fees"
	"github.com/vechain/thor/v2/rpc/logs"
	"github.com/vechain/thor/v2/rpc/simulation"
	"github.com/vechain/thor/v2/rpc/transactions"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

// EthRPCConfig holds configuration for the Ethereum JSON-RPC server.
type EthRPCConfig struct {
	// AllowedOrigins is the comma-separated list of allowed CORS origins.
	// Reuses the same value as the REST API --api-cors flag.
	AllowedOrigins string
	BacktraceLimit uint32
	CallGasLimit   uint64
	ClientVersion  string
}

// StartEthRPCServer starts the Ethereum JSON-RPC server on the given address.
// Returns the listening URL and a closer function.
func StartEthRPCServer(
	addr string,
	repo *chain.Repository,
	stater *state.Stater,
	txPool txpool.Pool,
	logDB *logdb.LogDB,
	forkConfig *thor.ForkConfig,
	config EthRPCConfig,
) (string, func(), error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, errors.Wrapf(err, "listen Eth RPC addr [%v]", addr)
	}

	chainID := thor.GetEthChainID(repo.GenesisBlock().Header().ID())

	d := rpc.NewDispatcher()
	rpcchain.New(repo, chainID, config.ClientVersion).Mount(d)
	blocks.New(repo, chainID).Mount(d)
	transactions.New(repo, chainID, txPool).Mount(d)
	accounts.New(repo, stater).Mount(d)
	logs.New(repo, logDB, config.BacktraceLimit).Mount(d)
	fees.New(repo, config.BacktraceLimit).Mount(d)
	simulation.New(repo, stater, forkConfig, config.CallGasLimit).Mount(d)

	origins := strings.Split(strings.TrimSpace(config.AllowedOrigins), ",")
	for i, o := range origins {
		origins[i] = strings.ToLower(strings.TrimSpace(o))
	}

	srv := rpc.New(d)

	corsHandler := handlers.CORS(
		handlers.AllowedOrigins(origins),
		handlers.AllowedHeaders([]string{"content-type"}),
		handlers.AllowedMethods([]string{"POST", "OPTIONS"}),
	)(srv)

	httpSrv := &http.Server{
		Handler:           corsHandler,
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       30 * time.Second,
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		httpSrv.Serve(listener) //nolint:errcheck
	})

	return "http://" + listener.Addr().String(), func() {
		httpSrv.Close()
		wg.Wait()
	}, nil
}

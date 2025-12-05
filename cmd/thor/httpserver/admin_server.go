// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package httpserver

import (
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api/admin"
	"github.com/vechain/thor/v2/api/admin/health"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/node"
	"github.com/vechain/thor/v2/comm"
)

func StartAdminServer(
	addr string,
	logLevel *slog.LevelVar,
	repo *chain.Repository,
	p2p *comm.Communicator,
	apiLogs *atomic.Bool,
	master *node.Master,
) (string, func(), error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, errors.Wrapf(err, "listen admin API addr [%v]", addr)
	}

	adminHandler := admin.NewHTTPHandler(logLevel, health.New(repo, p2p), apiLogs, master)

	srv := &http.Server{Handler: adminHandler, ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second}
	var goes sync.WaitGroup
	goes.Go(func() {
		srv.Serve(listener)
	})
	return "http://" + listener.Addr().String() + "/admin", func() {
		srv.Close()
		goes.Wait()
	}, nil
}

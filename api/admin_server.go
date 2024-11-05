// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/admin"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/co"
)

func StartAdminServer(addr string, logLevel *slog.LevelVar, repository *chain.Repository, node node.Network, blockInterval time.Duration) (string, func(), error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, errors.Wrapf(err, "listen admin API addr [%v]", addr)
	}

	adminHandler := admin.New(logLevel, repository, node, blockInterval)

	srv := &http.Server{Handler: adminHandler, ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second}
	var goes co.Goes
	goes.Go(func() {
		srv.Serve(listener)
	})
	return "http://" + listener.Addr().String() + "/admin", func() {
		srv.Close()
		goes.Wait()
	}, nil
}

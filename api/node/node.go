// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

type Node struct {
	pool         txpool.Pool
	nw           api.Network
	enableTxpool bool
}

func New(nw api.Network, pool txpool.Pool, enableTxpool bool) *Node {
	return &Node{
		pool,
		nw,
		enableTxpool,
	}
}

func (n *Node) PeersStats() []*api.PeerStats {
	return api.ConvertPeersStats(n.nw.PeersStats())
}

func (n *Node) handleNetwork(w http.ResponseWriter, _ *http.Request) error {
	return restutil.WriteJSON(w, n.PeersStats())
}

func (n *Node) handleGetTransactions(w http.ResponseWriter, req *http.Request) error {
	expanded, err := restutil.StringToBoolean(req.URL.Query().Get("expanded"), false)
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "expanded"))
	}

	originString := req.URL.Query().Get("origin")
	origin, err := restutil.StringToAddress(originString)
	if err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "origin"))
	}

	filteredTransactions := n.pool.Dump()
	if origin != nil {
		filteredTransactions, err = filterTransactions(*origin, filteredTransactions)
		if err != nil {
			return restutil.BadRequest(err)
		}
	}

	if expanded {
		trxs := make([]transactions.Transaction, len(filteredTransactions))
		for index, trx := range filteredTransactions {
			convertedTx := transactions.ConvertTransaction(trx, nil)
			trxs[index] = *convertedTx
		}

		return restutil.WriteJSON(w, trxs)
	}

	transactions := make([]thor.Bytes32, len(filteredTransactions))
	for index, tx := range filteredTransactions {
		transactions[index] = tx.ID()
	}

	return restutil.WriteJSON(w, transactions)
}

func (n *Node) handleGetTxpoolStatus(w http.ResponseWriter, req *http.Request) error {
	total := n.pool.Len()
	status := api.Status{
		Amount: uint(total),
	}
	return restutil.WriteJSON(w, status)
}

func (n *Node) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/network/peers").
		Methods(http.MethodGet).
		Name("GET /node/network/peers").
		HandlerFunc(restutil.WrapHandlerFunc(n.handleNetwork))

	if n.enableTxpool {
		sub.Path("/txpool").
			Methods(http.MethodGet).
			Name("GET /node/txpool").
			HandlerFunc(restutil.WrapHandlerFunc(n.handleGetTransactions))
		sub.Path("/txpool/status").
			Methods(http.MethodGet).
			Name("GET /node/txpool/status").
			HandlerFunc(restutil.WrapHandlerFunc(n.handleGetTxpoolStatus))
	}
}

func filterTransactions(origin thor.Address, allTransactions tx.Transactions) (tx.Transactions, error) {
	var filtered []*tx.Transaction

	for _, tx := range allTransactions {
		sender, err := tx.Origin()
		if err != nil {
			return nil, restutil.BadRequest(errors.WithMessage(err, "filtering origin"))
		}
		if sender == origin {
			filtered = append(filtered, tx)
		}
	}

	return filtered, nil
}

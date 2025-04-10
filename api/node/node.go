// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

type Node struct {
	pool *txpool.TxPool
	nw   Network
}

func New(nw Network, pool *txpool.TxPool) *Node {
	return &Node{
		pool,
		nw,
	}
}

func (n *Node) PeersStats() []*PeerStats {
	return ConvertPeersStats(n.nw.PeersStats())
}

func (n *Node) handleNetwork(w http.ResponseWriter, _ *http.Request) error {
	return utils.WriteJSON(w, n.PeersStats())
}

func filterTransactions(origin *thor.Address, allTransactions tx.Transactions) (tx.Transactions, error) {
	var filtered []*tx.Transaction

	for _, tx := range allTransactions {
		sender, err := tx.Origin()
		if err != nil {
			return nil, utils.BadRequest(errors.WithMessage(err, "filtering origin"))
		}
		if origin != nil && sender == *origin {
			filtered = append(filtered, tx)
		}
	}

	return filtered, nil
}

func (m *Node) handleGetTransactions(w http.ResponseWriter, req *http.Request) error {
	expanded, err := utils.StringToBoolean(req.URL.Query().Get("expanded"), false)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "expanded"))
	}

	originString := req.URL.Query().Get("origin")
	origin, err := utils.StringToAddress(originString)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "origin"))
	}

	var filteredTransactions tx.Transactions
	if origin == nil {
		filteredTransactions = m.pool.Dump()
	} else {
		filtered, err := filterTransactions(origin, m.pool.Dump())
		if err != nil {
			return utils.BadRequest(err)
		}
		filteredTransactions = filtered
	}

	if expanded {
		trxs := make([]transactions.Transaction, len(filteredTransactions))
		for index, tx := range filteredTransactions {
			origin, _ := tx.Origin()
			delegator, _ := tx.Delegator()

			txClauses := tx.Clauses()
			cls := make(transactions.Clauses, len(txClauses))
			for i, c := range txClauses {
				cls[i] = transactions.Clause{
					To:    c.To(),
					Value: math.HexOrDecimal256(*c.Value()),
					Data:  hexutil.Encode(c.Data()),
				}
			}
			br := tx.BlockRef()
			trxs[index] = transactions.Transaction{
				ChainTag:     tx.ChainTag(),
				ID:           tx.ID(),
				Origin:       origin,
				BlockRef:     hexutil.Encode(br[:]),
				Expiration:   tx.Expiration(),
				Nonce:        math.HexOrDecimal64(tx.Nonce()),
				Size:         uint32(tx.Size()),
				GasPriceCoef: tx.GasPriceCoef(),
				Gas:          tx.Gas(),
				DependsOn:    tx.DependsOn(),
				Clauses:      cls,
				Delegator:    delegator,
			}
		}

		return utils.WriteJSON(w, trxs)
	}

	transactions := make([]thor.Bytes32, len(filteredTransactions))
	for index, tx := range filteredTransactions {
		transactions[index] = tx.ID()
	}

	return utils.WriteJSON(w, transactions)
}

func (m *Node) handleGetTxpoolStatus(w http.ResponseWriter, req *http.Request) error {
	total := m.pool.Len()
	executables := m.pool.Executables()
	status := Status{
		Total:       uint(total),
		Executables: uint(len(executables)),
	}
	return utils.WriteJSON(w, status)
}

func (n *Node) Mount(root *mux.Router, pathPrefix string, enableTxpool bool) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/network/peers").
		Methods(http.MethodGet).
		Name("GET /node/network/peers").
		HandlerFunc(utils.WrapHandlerFunc(n.handleNetwork))

	if enableTxpool {
		sub.Path("/txpool").
			Methods(http.MethodGet).
			Name("GET /node/txpool").
			HandlerFunc(utils.WrapHandlerFunc(n.handleGetTransactions))
		sub.Path("/txpool/status").
			Methods(http.MethodGet).
			Name("GET /node/txpool/status").
			HandlerFunc(utils.WrapHandlerFunc(n.handleGetTxpoolStatus))
	}
}

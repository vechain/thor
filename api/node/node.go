// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"fmt"
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

func (m *Node) handleGetTransactions(w http.ResponseWriter, req *http.Request) error {
	expandedString := req.URL.Query().Get("expanded")
	if expandedString != "" && expandedString != "false" && expandedString != "true" {
		return utils.BadRequest(errors.WithMessage(errors.New("should be boolean"), "expanded"))
	}
	expanded := false
	if expandedString == "true" {
		expanded = true
	}

	fromString := req.URL.Query().Get("from")
	from := &thor.Address{}
	if fromString != "" {
		fromParsed, err := thor.ParseAddress(fromString)
		if err != nil {
			return utils.BadRequest(errors.WithMessage(err, "from"))
		}
		from = &fromParsed
	} else {
		from = nil
	}

	toString := req.URL.Query().Get("to")
	to := &thor.Address{}
	if toString != "" {
		toParsed, err := thor.ParseAddress(toString)
		if err != nil {
			return utils.BadRequest(errors.WithMessage(err, "to"))
		}
		to = &toParsed
	} else {
		to = nil
	}

	allTransactions := m.pool.Dump()
	if from != nil {
		var filtered []*tx.Transaction
		for _, tx := range allTransactions {
			sender, err := tx.Origin()
			if err != nil {
				return utils.BadRequest(errors.WithMessage(err, "filtering origin"))
			}
			if sender == *from {
				filtered = append(filtered, tx)
			}
		}
		allTransactions = filtered
	}

	if to != nil {
		var filtered []*tx.Transaction
		for _, tx := range allTransactions {
			clauses := tx.Clauses()
			toAdd := false
			for _, cl := range clauses {
				toClause := cl.To()
				if *toClause == *to {
					toAdd = true
					break
				}
			}

			if toAdd {
				filtered = append(filtered, tx)
			}
		}
		allTransactions = filtered
	}

	if expanded {
		trxs := make([]transactions.Transaction, len(allTransactions))
		for index, tx := range allTransactions {
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
			gasPriceCoef := tx.GasPriceCoef()
			trxs[index] = transactions.Transaction{
				ChainTag:     tx.ChainTag(),
				ID:           tx.ID(),
				Origin:       origin,
				BlockRef:     hexutil.Encode(br[:]),
				Expiration:   tx.Expiration(),
				Nonce:        math.HexOrDecimal64(tx.Nonce()),
				Size:         uint32(tx.Size()),
				GasPriceCoef: gasPriceCoef,
				Gas:          tx.Gas(),
				DependsOn:    tx.DependsOn(),
				Clauses:      cls,
				Delegator:    delegator,
			}
		}

		return utils.WriteJSON(w, trxs)
	}

	transactions := make([]thor.Bytes32, len(allTransactions))
	for index, tx := range allTransactions {
		hash := tx.Hash()
		transactions[index] = hash
	}

	return utils.WriteJSON(w, transactions)

}

func (m *Node) handleGetMempoolStatus(w http.ResponseWriter, req *http.Request) error {
	total := m.pool.Len()
	executables := m.pool.Executables()
	status := Status{
		Total:       uint(total),
		Executables: uint(len(executables)),
	}
	return utils.WriteJSON(w, status)
}

func (n *Node) Mount(root *mux.Router, pathPrefix string, enableMempool bool) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/network/peers").
		Methods(http.MethodGet).
		Name("GET /node/network/peers").
		HandlerFunc(utils.WrapHandlerFunc(n.handleNetwork))

	fmt.Println("EnableMempool", enableMempool)
	if enableMempool {
		sub.Path("/mempool").
			Methods(http.MethodGet).
			Name("GET /node/mempool").
			HandlerFunc(utils.WrapHandlerFunc(n.handleGetTransactions))
		sub.Path("/mempool/status").
			Methods(http.MethodGet).
			Name("GET /node/mempool/status").
			HandlerFunc(utils.WrapHandlerFunc(n.handleGetMempoolStatus))
	}
}

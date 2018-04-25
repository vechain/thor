package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/accounts"
	"github.com/vechain/thor/api/blocks"
	"github.com/vechain/thor/api/logs"
	"github.com/vechain/thor/api/node"
	"github.com/vechain/thor/api/transactions"
	"github.com/vechain/thor/api/transfers"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/transferdb"
	"github.com/vechain/thor/txpool"
)

//New return api router
func New(chain *chain.Chain, stateCreator *state.Creator, txPool *txpool.TxPool, logDB *logdb.LogDB, transferDB *transferdb.TransferDB, nw node.Network) http.HandlerFunc {
	router := mux.NewRouter()
	serveStatic(router, "/static")
	accounts.New(chain, stateCreator).Mount(router, "/accounts")
	logs.New(logDB).Mount(router, "/logs")
	transfers.New(transferDB).Mount(router, "/transfers")
	blocks.New(chain).Mount(router, "/blocks")
	transactions.New(chain, txPool, transferDB).Mount(router, "/transactions")
	node.New(nw).Mount(router, "/node")
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		router.ServeHTTP(w, req)
		fmt.Println(w)
	}
}

func serveStatic(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/static").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir(""))))
}

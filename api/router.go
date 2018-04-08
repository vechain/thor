package api

import (
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/accounts"
	"github.com/vechain/thor/api/blocks"
	"github.com/vechain/thor/api/transactions"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/logdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/txpool"
)

//New return api router
func New(chain *chain.Chain, stateCreator *state.Creator, txPool *txpool.TxPool, logDB *logdb.LogDB) *mux.Router {
	router := mux.NewRouter()
	accounts.New(chain, stateCreator, logDB).Mount(router, "/accounts")
	blocks.New(chain).Mount(router, "/blocks")
	transactions.New(chain, txPool).Mount(router, "/transactions")
	return router
}

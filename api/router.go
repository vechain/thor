package api

import (
	"github.com/gorilla/mux"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
)

//NewHTTPHandler return api router
func NewHTTPHandler(chain *chain.Chain, stateCreator *state.Creator) *mux.Router {
	router := mux.NewRouter()
	NewAccountHTTPRouter(router, NewAccountInterface(chain, stateCreator))
	NewTransactionHTTPRouter(router, NewTransactionInterface(chain))
	NewBlockHTTPRouter(router, NewBlockInterface(chain))
	return router
}

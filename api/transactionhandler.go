package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils/httpx"
	"github.com/vechain/thor/thor"
)

//TransactionHTTPPathPrefix http path prefix
const TransactionHTTPPathPrefix = "/transactions"

//NewTransactionHTTPRouter add path to router
func NewTransactionHTTPRouter(router *mux.Router, ti *TransactionInterface) {
	sub := router.PathPrefix(TransactionHTTPPathPrefix).Subrouter()

	sub.Path("/{id}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ti.handleGetTransactionByID))
}

func (ti *TransactionInterface) handleGetTransactionByID(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if len(query) == 0 {
		return httpx.Error(" No Params! ", 400)
	}
	id, ok := query["id"]
	if !ok {
		return httpx.Error(" Invalid Params! ", 400)
	}
	txID, err := thor.ParseHash(id)
	if err != nil {
		return httpx.Error(" Invalid hash! ", 400)
	}
	tx, err := ti.GetTransactionByID(txID)
	if err != nil {
		return httpx.Error(" Get transaction failed! ", 400)
	}
	txData, err := json.Marshal(tx)
	if err != nil {
		return httpx.Error(" System Error! ", 400)
	}
	w.Write(txData)
	return nil
}

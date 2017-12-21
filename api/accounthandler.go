package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/api/utils/httpx"
)

//HTTPPathPrefix http path prefix
const HTTPPathPrefix = "/account"

//NewHTTPRouter add path to router
func NewHTTPRouter(router *mux.Router, accountManager *AccountManager) {
	sub := router.PathPrefix(HTTPPathPrefix).Subrouter()
	sub.Path("/address/{address}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(accountManager.handleGetAccount))
}
func (am *AccountManager) handleGetAccount(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if query == nil {
		w.WriteHeader(400)
	}
	addr, ok := query["address"]
	if !ok {
		w.WriteHeader(400)
	}
	fmt.Println("query :", query)
	address, err := acc.ParseAddress(addr)

	if err != nil {
		w.WriteHeader(400)
	}
	account := am.GetAccount(*address)
	str, err := json.Marshal(account)
	if err != nil {
		return err
	}
	w.Write(str)
	return nil
}

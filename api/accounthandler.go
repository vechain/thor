package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/api/utils/httpx"
)

//AccountHTTPPathPrefix http path prefix
const AccountHTTPPathPrefix = "/account"

//NewAccountHTTPRouter add path to router
func NewAccountHTTPRouter(router *mux.Router, ai *AccountInterface) {
	sub := router.PathPrefix(AccountHTTPPathPrefix).Subrouter()
	sub.Path("/address/{address}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ai.handleGetAccount))
}
func (ai *AccountInterface) handleGetAccount(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if query == nil {
		return httpx.Error(errors.New(" No Params! "), 400)
	}
	addr, ok := query["address"]
	if !ok {
		return httpx.Error(errors.New(" Invalid Params! "), 400)
	}
	address, err := acc.ParseAddress(addr)
	if err != nil {
		return httpx.Error(errors.New(" Parse address failed! "), 400)
	}
	account := ai.GetAccount(*address)
	str, err := json.Marshal(account)
	if err != nil {
		return httpx.Error(errors.New(" System Error! "), 500)
	}
	w.Write(str)
	return nil
}

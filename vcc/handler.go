package vcc

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/solidb/utils/httpx"
	"github.com/vechain/thor/acc"
)

//HTTPPathPrefix http path prefix
const HTTPPathPrefix = "/account/"

//NewHTTPRouter add path to router
func NewHTTPRouter(router *mux.Router, accountManager *AccountManager) {
	sub := router.PathPrefix(HTTPPathPrefix).Subrouter()
	sub.Path("/address/{address}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(accountManager.handleGetAccount))
}
func (accountManager *AccountManager) handleGetAccount(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	fmt.Println("query :", query)
	address, err := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e01a")
	account := accountManager.GetAccount(*address)
	str, err := json.Marshal(account)
	if err != nil {
		return err
	}
	w.Write(str)
	return nil
}

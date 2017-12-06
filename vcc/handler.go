package vcc

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/solidb/utils/httpx"
	"github.com/vechain/vecore/acc"
)

//HTTPPathPrefix http path prefix
const HTTPPathPrefix = "/account/"

//NewHTTPRouter add path to router
func NewHTTPRouter(router *mux.Router, accountManager *AccountManager) {
	sub := router.PathPrefix(HTTPPathPrefix).Subrouter()
	sub.Path("/address/{address}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(accountManager.handleGetAccount))
}
func (accountManager *AccountManager) handleGetAccount(w http.ResponseWriter, req *http.Request) error {
	fmt.Println(req)
	query := mux.Vars(req)
	fmt.Println("query:", query)
	address, err := acc.ParseAddress("56e81f171bcc55a6ff8345e692c0f86e5b48e01a")
	fmt.Println("address err accountManager:", address, err, accountManager)
	account := accountManager.GetAccount(*address)
	fmt.Println("account", account)
	str, err := json.Marshal(account)
	fmt.Println("str err :", str, err)
	if err != nil {
		return err
	}
	var data interface{}
	json.Unmarshal(str, &data)
	fmt.Println("data:", data)
	w.Write(str)
	return nil
}

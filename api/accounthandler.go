package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils/httpx"
	"github.com/vechain/thor/thor"
)

//AccountHTTPPathPrefix http path prefix
const AccountHTTPPathPrefix = "/account"

//NewAccountHTTPRouter add path to router
func NewAccountHTTPRouter(router *mux.Router, ai *AccountInterface) {
	sub := router.PathPrefix(AccountHTTPPathPrefix).Subrouter()

	sub.Path("/address/{address}/balance").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ai.handleGetBalance))
	sub.Path("/address/{address}/code").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ai.handleGetCode))
	sub.Path("/address/{address}/key/{key}/storage").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ai.handleGetStorage))
}

func (ai *AccountInterface) handleGetBalance(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if len(query) == 0 {
		return httpx.Error(" No Params! ", 400)
	}
	addr, ok := query["address"]
	if !ok {
		return httpx.Error(" Invalid Params! ", 400)
	}
	address, err := thor.ParseAddress(addr)
	if err != nil {
		return httpx.Error(" Invalid address! ", 400)
	}
	b := ai.GetBalance(address)
	str, err := json.Marshal(b)
	if err != nil {
		return httpx.Error(" System Error! ", 500)
	}
	w.Write(str)
	return nil
}

func (ai *AccountInterface) handleGetCode(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if len(query) == 0 {
		return httpx.Error(" No Params! ", 400)
	}
	addr, ok := query["address"]
	if !ok {
		return httpx.Error(" Invalid Params! ", 400)
	}
	address, err := thor.ParseAddress(addr)
	if err != nil {
		return httpx.Error(" Invalid address! ", 400)
	}
	c := ai.GetCode(address)
	str, err := json.Marshal(c)
	if err != nil {
		return httpx.Error(" System Error! ", 500)
	}
	w.Write(str)
	return nil
}

func (ai *AccountInterface) handleGetStorage(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if len(query) == 0 {
		return httpx.Error(" No Params! ", 400)
	}
	addr, ok := query["address"]
	if !ok {
		return httpx.Error(" Invalid Params! ", 400)
	}
	key, ok := query["key"]
	if !ok {
		return httpx.Error(" Invalid Params! ", 400)
	}
	address, err := thor.ParseAddress(addr)
	if err != nil {
		return httpx.Error(" Invalid address! ", 400)
	}
	keyhash, err := thor.ParseHash(key)
	if err != nil {
		return httpx.Error(" Invalid key! ", 400)
	}
	v := ai.GetStorage(address, keyhash)
	str, err := json.Marshal(v)
	if err != nil {
		return httpx.Error(" System Error! ", 500)
	}
	w.Write(str)
	return nil
}

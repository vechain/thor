package api

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils/httpx"
	"github.com/vechain/thor/thor"
	"math/big"
	"net/http"
)

//AccountHTTPPathPrefix http path prefix
const AccountHTTPPathPrefix = "/account"

//NewAccountHTTPRouter add path to router
func NewAccountHTTPRouter(router *mux.Router, ai *AccountInterface) {
	sub := router.PathPrefix(AccountHTTPPathPrefix).Subrouter()

	sub.Path("/balance/{address}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ai.handleGetBalance))
	sub.Path("/code/{address}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ai.handleGetCode))
	sub.Path("/storage/{address}/{key}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ai.handleGetStorage))
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
	data := map[string]*big.Int{
		"balance": b,
	}

	d, err := json.Marshal(data)
	if err != nil {
		return httpx.Error(" System Error! ", 500)
	}
	w.Write(d)
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

	code := ai.GetCode(address)
	data := map[string][]byte{
		"code": code,
	}

	d, err := json.Marshal(data)
	if err != nil {
		return httpx.Error(" System Error! ", 500)
	}
	w.Write(d)
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

	value := ai.GetStorage(address, keyhash)
	data := map[string]string{
		key: value.String(),
	}
	d, err := json.Marshal(data)
	if err != nil {
		return httpx.Error(" System Error! ", 500)
	}
	w.Write(d)
	return nil
}

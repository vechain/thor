package api

import (
	"encoding/json"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils/httpx"
	"github.com/vechain/thor/thor"
	"net/http"
)

//AccountHTTPPathPrefix http path prefix
const AccountHTTPPathPrefix = "/accounts"

//NewAccountHTTPRouter add path to router
func NewAccountHTTPRouter(router *mux.Router, ai *AccountInterface) {
	sub := router.PathPrefix(AccountHTTPPathPrefix).Subrouter()

	sub.Path("/{address}/balance").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ai.handleGetBalance))
	sub.Path("/{address}/code").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ai.handleGetCode))
	sub.Path("/{address}/storage").Queries("key", "{key}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ai.handleGetStorage))
}
func (ai *AccountInterface) handleGetBalance(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if len(query) == 0 {
		return httpx.Error("No Params!", 400)
	}
	addr, ok := query["address"]
	if !ok {
		return httpx.Error("Invalid Params!", 400)
	}
	address, err := thor.ParseAddress(addr)
	if err != nil {
		return httpx.Error("Invalid address!", 400)
	}

	balance := ai.GetBalance(address)
	dataMap := make(map[string]interface{})
	dataMap["result"] = balance
	data, err := json.Marshal(dataMap)
	if err != nil {
		return httpx.Error("System Error!", 500)
	}
	w.Write(data)
	return nil
}
func (ai *AccountInterface) handleGetCode(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if len(query) == 0 {
		return httpx.Error("No Params!", 400)
	}
	addr, ok := query["address"]
	if !ok {
		return httpx.Error("Invalid Params!", 400)
	}
	address, err := thor.ParseAddress(addr)
	if err != nil {
		return httpx.Error("Invalid address!", 400)
	}

	code := ai.GetCode(address)
	dataMap := make(map[string]interface{})
	dataMap["result"] = hexutil.Encode(code)
	data, err := json.Marshal(dataMap)
	if err != nil {
		return httpx.Error("System Error!", 500)
	}
	w.Write(data)
	return nil
}

func (ai *AccountInterface) handleGetStorage(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if len(query) == 0 {
		return httpx.Error("No Params!", 400)
	}
	addr, ok := query["address"]
	if !ok {
		return httpx.Error("Invalid Params!", 400)
	}
	key, ok := query["key"]
	if !ok {
		return httpx.Error("Invalid Params!", 400)
	}
	address, err := thor.ParseAddress(addr)
	if err != nil {
		return httpx.Error("Invalid address!", 400)
	}
	keyhash, err := thor.ParseHash(key)
	if err != nil {
		return httpx.Error("Invalid key!", 400)
	}

	value := ai.GetStorage(address, keyhash)
	storage := map[string]string{
		"result": value.String(),
	}
	data, err := json.Marshal(storage)
	if err != nil {
		return httpx.Error("System Error!", 500)
	}
	w.Write(data)
	return nil
}

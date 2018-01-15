package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils/httpx"
	"github.com/vechain/thor/thor"
)

//TransactionHTTPPathPrefix http path prefix
const TransactionHTTPPathPrefix = "/transaction"

//NewTransactionHTTPRouter add path to router
func NewTransactionHTTPRouter(router *mux.Router, ti *TransactionInterface) {
	sub := router.PathPrefix(TransactionHTTPPathPrefix).Subrouter()

	sub.Path("/hash/{hash}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ti.handleGetTransactionByHash))
	sub.Path("/blocknumber/{number}/txindex/{index}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(ti.handleGetTransactionFromBlock))
}

func (ti *TransactionInterface) handleGetTransactionByHash(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if len(query) == 0 {
		return httpx.Error(" No Params! ", 400)
	}
	hashstring, ok := query["hash"]
	if !ok {
		return httpx.Error(" Invalid Params! ", 400)
	}
	hash, err := thor.ParseHash(hashstring)
	if err != nil {
		return httpx.Error(" Invalid hash! ", 400)
	}
	tx, err := ti.GetTransactionByHash(hash)
	if err != nil {
		return httpx.Error(" Get transaction failed! ", 400)
	}
	str, err := json.Marshal(tx)
	if err != nil {
		return httpx.Error(" System Error! ", 400)
	}
	w.Write(str)
	return nil
}

func (ti *TransactionInterface) handleGetTransactionFromBlock(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if query == nil {
		return httpx.Error(" No Params! ", 400)
	}

	number, ok := query["number"]
	if !ok {
		return httpx.Error(" Invalid Params! ", 400)
	}
	index, ok := query["index"]
	if !ok {
		return httpx.Error(" Invalid Params! ", 400)
	}

	bn, err := strconv.Atoi(number)
	if err != nil {
		return httpx.Error(" Parse block number failed! ", 400)
	}
	txIndex, err := strconv.Atoi(index)
	if err != nil {
		return httpx.Error(" Parse transaction index failed! ", 400)
	}

	tx, err := ti.GetTransactionFromBlock(uint32(bn), uint64(txIndex))
	if err != nil {
		return httpx.Error(" Get transaction failed! ", 400)
	}
	str, err := json.Marshal(tx)
	if err != nil {
		return httpx.Error(" System Error! ", 400)
	}
	w.Write(str)
	return nil
}

package api

import (
	"encoding/json"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils/httpx"
	"github.com/vechain/thor/thor"
	"net/http"
)

//ContractHTTPPathPrefix http path prefix
const ContractHTTPPathPrefix = "/contracts"

//NewContractHTTPRouter add path to router
func NewContractHTTPRouter(router *mux.Router, ci *ContractInterface) {
	sub := router.PathPrefix(ContractHTTPPathPrefix).Subrouter()

	sub.Path("/{contractAddr}").Methods("POST").HandlerFunc(httpx.WrapHandlerFunc(ci.handleCallContract))

}

func (ci *ContractInterface) handleCallContract(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if len(query) == 0 {
		return httpx.Error("No Params!", 400)
	}
	contractAddr, ok := query["contractAddr"]
	if !ok {
		return httpx.Error("No contract address!", 400)
	}
	addr, err := thor.ParseAddress(contractAddr)
	if err != nil {
		return httpx.Error("Invalid contract address!", 400)
	}
	optionData := []byte(req.FormValue("options"))
	options := new(ContractInterfaceOptions)
	if err := json.Unmarshal(optionData, &options); err != nil {
		return err
	}
	input := []byte(req.FormValue("input"))
	output, err := ci.Call(&addr, input, options)
	if err != nil {
		return httpx.Error("Call contract failed!", 400)
	}
	dataMap := map[string]string{
		"result": hexutil.Encode(output),
	}
	return httpx.ResponseJSON(w, dataMap)
}

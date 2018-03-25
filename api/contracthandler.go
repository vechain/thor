package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/thor"
)

//ContractHTTPPathPrefix http path prefix
const ContractHTTPPathPrefix = "/contracts"

//NewContractHTTPRouter add path to router
func NewContractHTTPRouter(router *mux.Router, ci *ContractInterface) {
	sub := router.PathPrefix(ContractHTTPPathPrefix).Subrouter()

	sub.Path("/{contractAddr}").Methods("POST").HandlerFunc(WrapHandlerFunc(ci.handleCallContract))

}

func (ci *ContractInterface) handleCallContract(w http.ResponseWriter, req *http.Request) error {
	body, _ := ioutil.ReadAll(req.Body)
	req.Body.Close()

	query := mux.Vars(req)
	if len(query) == 0 {
		return Error("No Params!", 400)
	}
	contractAddr, ok := query["contractAddr"]
	if !ok {
		return Error("No contract address!", 400)
	}
	addr, err := thor.ParseAddress(contractAddr)
	if err != nil {
		return Error("Invalid contract address!", 400)
	}

	interfaceBody := &ContractCallBody{}
	if err := json.Unmarshal(body, &interfaceBody); err != nil {
		return err
	}

	output, err := ci.Call(&addr, interfaceBody)
	if err != nil {
		return Error("Call contract failed!", 400)
	}
	dataMap := map[string]string{
		"result": hexutil.Encode(output),
	}
	return ResponseJSON(w, dataMap)
}

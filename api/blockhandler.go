package api

import (
	"github.com/gorilla/mux"
	"github.com/vechain/thor/thor"
	"math/big"
	"net/http"
)

//BlockHTTPPathPrefix http path prefix
const BlockHTTPPathPrefix = "/blocks"

//NewBlockHTTPRouter add path to router
func NewBlockHTTPRouter(router *mux.Router, bi *BlockInterface) {
	sub := router.PathPrefix(BlockHTTPPathPrefix).Subrouter()

	sub.Path("").Queries("number", "{number:[0-9]+}").Methods("GET").HandlerFunc(WrapHandlerFunc(bi.handleGetBlockByNumber))
	sub.Path("/best").Methods("GET").HandlerFunc(WrapHandlerFunc(bi.handleGetBestBlock))
	sub.Path("/{id}").Methods("GET").HandlerFunc(WrapHandlerFunc(bi.handleGetBlockByID))

}

func (bi *BlockInterface) handleGetBlockByID(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if len(query) == 0 {
		return Error("No Params!", 400)
	}
	id, ok := query["id"]
	if !ok {
		return Error("Invalid Params!", 400)
	}
	blkID, err := thor.ParseHash(id)
	if err != nil {
		return Error("Invalid blockhash!", 400)
	}
	block, err := bi.GetBlockByID(blkID)
	if err != nil {
		return Error("Block not found!", 400)
	}
	return ResponseJSON(w, block)
}

func (bi *BlockInterface) handleGetBlockByNumber(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if query == nil {
		return Error("No Params!", 400)
	}
	number, ok := new(big.Int).SetString(query["number"], 10)
	if !ok {
		return Error("Invalid Number!", 400)
	}
	block, err := bi.GetBlockByNumber(uint32(number.Int64()))
	if err != nil {
		return Error("Get block failed!", 400)
	}
	return ResponseJSON(w, block)
}

func (bi *BlockInterface) handleGetBestBlock(w http.ResponseWriter, req *http.Request) error {
	block, err := bi.GetBestBlock()
	if err != nil {
		return Error("Block not found!", 400)
	}
	return ResponseJSON(w, block)
}

package api

import (
	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils/httpx"
	"github.com/vechain/thor/thor"
	"math/big"
	"net/http"
)

//BlockHTTPPathPrefix http path prefix
const BlockHTTPPathPrefix = "/blocks"

//NewBlockHTTPRouter add path to router
func NewBlockHTTPRouter(router *mux.Router, bi *BlockInterface) {
	sub := router.PathPrefix(BlockHTTPPathPrefix).Subrouter()

	sub.Path("").Queries("number", "{number:[0-9]+}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(bi.handleGetBlockByNumber))
	sub.Path("/best").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(bi.handleGetBestBlock))
	sub.Path("/{id}").Methods("GET").HandlerFunc(httpx.WrapHandlerFunc(bi.handleGetBlockByID))

}

func (bi *BlockInterface) handleGetBlockByID(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if len(query) == 0 {
		return httpx.Error("No Params!", 400)
	}
	id, ok := query["id"]
	if !ok {
		return httpx.Error("Invalid Params!", 400)
	}
	blkID, err := thor.ParseHash(id)
	if err != nil {
		return httpx.Error("Invalid blockhash!", 400)
	}
	block, err := bi.GetBlockByID(blkID)
	if err != nil {
		return httpx.Error("Block not found!", 400)
	}
	return httpx.ResponseJSON(w, block)
}

func (bi *BlockInterface) handleGetBlockByNumber(w http.ResponseWriter, req *http.Request) error {
	query := mux.Vars(req)
	if query == nil {
		return httpx.Error("No Params!", 400)
	}
	number, ok := new(big.Int).SetString(query["number"], 10)
	if !ok {
		return httpx.Error("Invalid Number!", 400)
	}
	block, err := bi.GetBlockByNumber(uint32(number.Int64()))
	if err != nil {
		return httpx.Error("Get block failed!", 400)
	}
	return httpx.ResponseJSON(w, block)
}

func (bi *BlockInterface) handleGetBestBlock(w http.ResponseWriter, req *http.Request) error {
	block, err := bi.GetBestBlock()
	if err != nil {
		return httpx.Error("Block not found!", 400)
	}
	return httpx.ResponseJSON(w, block)
}

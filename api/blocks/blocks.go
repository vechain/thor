package blocks

import (
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type Blocks struct {
	chain *chain.Chain
}

func New(chain *chain.Chain) *Blocks {
	return &Blocks{
		chain,
	}
}

func (b *Blocks) getBlockByID(blockID thor.Hash) (*Block, error) {
	blk, err := b.chain.GetBlock(blockID)
	if err != nil {
		if b.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return ConvertBlock(blk), nil
}

func (b *Blocks) getBlockByNumber(blockNumber uint32) (*Block, error) {
	blk, err := b.chain.GetBlockByNumber(blockNumber)
	if err != nil {
		if b.chain.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return ConvertBlock(blk), nil
}

func (b *Blocks) getBestBlock() (*Block, error) {
	blk, err := b.chain.GetBestBlock()
	if err != nil {
		return nil, err
	}
	return ConvertBlock(blk), nil
}

func (b *Blocks) handleGetBlockByID(w http.ResponseWriter, req *http.Request) error {
	id := mux.Vars(req)["id"]
	blkID, err := thor.ParseHash(id)
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "id"), http.StatusBadRequest)
	}
	block, err := b.getBlockByID(blkID)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, block)
}

func (b *Blocks) handleGetBlockByNumber(w http.ResponseWriter, req *http.Request) error {
	blockNum, err := b.getBlockNumberByString(req.URL.Query().Get("number"))
	if err != nil {
		return utils.HTTPError(errors.Wrap(err, "blockNumber"), http.StatusBadRequest)
	}
	block, err := b.getBlockByNumber(blockNum)
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, block)
}

func (b *Blocks) handleGetBestBlock(w http.ResponseWriter, req *http.Request) error {
	block, err := b.getBestBlock()
	if err != nil {
		return utils.HTTPError(err, http.StatusBadRequest)
	}
	return utils.WriteJSON(w, block)
}

func (b *Blocks) getBlockNumberByString(blkNum string) (uint32, error) {
	if blkNum == "" {
		return math.MaxUint32, nil
	}
	n, err := strconv.ParseInt(blkNum, 0, 0)
	if err != nil {
		return math.MaxUint32, err
	}
	if n > math.MaxUint32 {
		return math.MaxUint32, errors.New("block number exceeded")
	}
	return uint32(n), nil
}

func (b *Blocks) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("/best").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(b.handleGetBestBlock))
	sub.Path("/{id}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(b.handleGetBlockByID))
	sub.Path("").Queries("number", "{number:[0-9]+}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(b.handleGetBlockByNumber))

}

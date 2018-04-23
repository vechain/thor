package blocks

import (
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/block"
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

func (b *Blocks) handleGetBlock(w http.ResponseWriter, req *http.Request) error {
	revision := mux.Vars(req)["revision"]
	block, err := b.getBlock(revision)
	if err != nil {
		return utils.BadRequest(err, "revision")
	}
	blk, err := ConvertBlock(block)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, blk)
}

func (b *Blocks) getBlock(revision string) (*block.Block, error) {
	if revision == "" || revision == "best" {
		return b.chain.GetBestBlock()
	}
	blkID, err := thor.ParseBytes32(revision)
	if err != nil {
		n, err := strconv.ParseUint(revision, 0, 0)
		if err != nil {
			return nil, err
		}
		if n > math.MaxUint32 {
			return nil, errors.New("block number exceeded")
		}
		return b.chain.GetBlockByNumber(uint32(n))
	}
	return b.chain.GetBlock(blkID)
}

func (b *Blocks) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/{revision}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(b.handleGetBlock))

}

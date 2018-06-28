// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

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
	revision, err := b.parseRevision(mux.Vars(req)["revision"])
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	block, err := b.getBlock(revision)
	if err != nil {
		if b.chain.IsNotFound(err) {
			return utils.WriteJSON(w, nil)
		}
		return err
	}
	isTrunk, err := b.isTrunk(block.Header().ID(), block.Header().Number())
	if err != nil {
		return err
	}
	blk, err := convertBlock(block, isTrunk)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, blk)
}

func (b *Blocks) parseRevision(revision string) (interface{}, error) {
	if revision == "" || revision == "best" {
		return nil, nil
	}
	if len(revision) == 66 || len(revision) == 64 {
		blockID, err := thor.ParseBytes32(revision)
		if err != nil {
			return nil, err
		}
		return blockID, nil
	}
	n, err := strconv.ParseUint(revision, 0, 0)
	if err != nil {
		return nil, err
	}
	if n > math.MaxUint32 {
		return nil, errors.New("block number out of max uint32")
	}
	return uint32(n), err
}

func (b *Blocks) getBlock(revision interface{}) (*block.Block, error) {
	switch revision.(type) {
	case thor.Bytes32:
		return b.chain.GetBlock(revision.(thor.Bytes32))
	case uint32:
		return b.chain.GetTrunkBlock(revision.(uint32))
	default:
		return b.chain.BestBlock(), nil
	}
}

func (b *Blocks) isTrunk(blkID thor.Bytes32, blkNum uint32) (bool, error) {
	best := b.chain.BestBlock()
	ancestorID, err := b.chain.GetAncestorBlockID(best.Header().ID(), blkNum)
	if err != nil {
		return false, err
	}
	return ancestorID == blkID, nil
}

func (b *Blocks) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/{revision}").Methods("GET").HandlerFunc(utils.WrapHandlerFunc(b.handleGetBlock))

}

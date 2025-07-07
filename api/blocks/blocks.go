// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks

import (
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

type Blocks struct {
	repo *chain.Repository
	bft  bft.Committer
}

func New(repo *chain.Repository, bft bft.Committer) *Blocks {
	return &Blocks{
		repo,
		bft,
	}
}

func (b *Blocks) handleGetBlock(w http.ResponseWriter, req *http.Request) error {
	revision, err := utils.ParseRevision(mux.Vars(req)["revision"], false)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	raw, err := utils.StringToBoolean(req.URL.Query().Get("raw"), false)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "raw"))
	}
	expanded, err := utils.StringToBoolean(req.URL.Query().Get("expanded"), false)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "expanded"))
	}

	if raw && expanded {
		return utils.BadRequest(errors.WithMessage(errors.New("Raw and Expanded are mutually exclusive"), "raw&expanded"))
	}

	summary, err := utils.GetSummary(revision, b.repo, b.bft)
	if err != nil {
		if b.repo.IsNotFound(err) {
			return utils.WriteJSON(w, nil)
		}
		return err
	}

	if raw {
		rlpEncoded, err := rlp.EncodeToBytes(summary.Header)
		if err != nil {
			return err
		}
		return utils.WriteJSON(w, &api.JSONRawBlockSummary{
			Raw: fmt.Sprintf("0x%s", hex.EncodeToString(rlpEncoded)),
		})
	}

	isTrunk, err := b.isTrunk(summary.Header.ID(), summary.Header.Number())
	if err != nil {
		return err
	}

	var isFinalized bool
	if isTrunk {
		finalized := b.bft.Finalized()
		if block.Number(finalized) >= summary.Header.Number() {
			isFinalized = true
		}
	}

	jSummary := api.BuildJSONBlockSummary(summary, isTrunk, isFinalized)
	if expanded {
		txs, err := b.repo.GetBlockTransactions(summary.Header.ID())
		if err != nil {
			return err
		}
		receipts, err := b.repo.GetBlockReceipts(summary.Header.ID())
		if err != nil {
			return err
		}

		return utils.WriteJSON(w, &api.JSONExpandedBlock{
			JSONBlockSummary: jSummary,
			Transactions:     api.BuildJSONEmbeddedTxs(txs, receipts),
		})
	}

	return utils.WriteJSON(w, &api.JSONCollapsedBlock{
		JSONBlockSummary: jSummary,
		Transactions:     summary.Txs,
	})
}

func (b *Blocks) isTrunk(blkID thor.Bytes32, blkNum uint32) (bool, error) {
	idByNum, err := b.repo.NewBestChain().GetBlockID(blkNum)
	if err != nil {
		return false, err
	}
	return blkID == idByNum, nil
}

func (b *Blocks) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()
	sub.Path("/{revision}").
		Methods(http.MethodGet).
		Name("GET /blocks/{revision}").
		HandlerFunc(utils.WrapHandlerFunc(b.handleGetBlock))
}

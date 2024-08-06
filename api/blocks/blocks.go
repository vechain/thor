// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks

import (
	"fmt"
	"math"
	"math/big"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

type Blocks struct {
	repo *chain.Repository
	bft  bft.Finalizer
}

func New(repo *chain.Repository, bft bft.Finalizer) *Blocks {
	return &Blocks{
		repo,
		bft,
	}
}

func (b *Blocks) handleGetGasCoef(w http.ResponseWriter, req *http.Request) error {
	revision, err := utils.ParseRevision(mux.Vars(req)["revision"], false)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	expanded := req.URL.Query().Get("expanded")
	if expanded != "" && expanded != "false" && expanded != "true" {
		return utils.BadRequest(errors.WithMessage(errors.New("should be boolean"), "expanded"))
	}

	summary, err := utils.GetSummary(revision, b.repo, b.bft)
	if err != nil {
		if b.repo.IsNotFound(err) {
			return utils.WriteJSON(w, nil)
		}
		return err
	}

	txs, err := b.repo.GetBlockTransactions(summary.Header.ID())
	if err != nil {
		return err
	}

	// Initialize avgBaseCoef as a big.Int with a value of 0
	avgBaseCoef := big.NewInt(0)

	// Calculate average base coefficient
	if len(txs) > 0 {
		for _, tx := range txs {
			avgBaseCoef = avgBaseCoef.Add(avgBaseCoef, big.NewInt(int64(tx.GasPriceCoef())))
		}
		avgBaseCoef = avgBaseCoef.Div(avgBaseCoef, big.NewInt(int64(len(txs))))
	} else {
		fmt.Println("No transactions")
	}

	totalGasUsed := summary.Header.GasUsed()
	gasLimit := summary.Header.GasLimit()

	div := float64(totalGasUsed) / float64(gasLimit)

	// Convert avgBaseCoef to big.Float for percentage calculation
	avgBaseCoefFloat := new(big.Float).SetInt(avgBaseCoef)
	multiplier := big.NewFloat(0.12)
	twelvePercent := new(big.Float).Mul(avgBaseCoefFloat, multiplier)

	// Check if the gas usage is more than 50%
	suggestedBaseCoef := avgBaseCoefFloat

	// todo remove reason
	reason := fmt.Sprintf("Blk: %s, GasLimit: %d, GasUsed: %d", summary.Header.ID().AbbrevString(), summary.Header.GasLimit(), summary.Header.GasUsed())
	if div > 0.5 {
		reason += " - Increased!"
		suggestedBaseCoef = suggestedBaseCoef.Add(suggestedBaseCoef, twelvePercent)
	} else if div < 0.5 {
		reason += " - Decreased!"
		suggestedBaseCoef = suggestedBaseCoef.Sub(suggestedBaseCoef, twelvePercent)
	} else {
		reason += " - Stayed the same."
	}

	// Convert big.Float to uint8 with rounding up
	avgBaseCoefFloat64, _ := avgBaseCoefFloat.Float64()
	suggestedBaseCoefFloat64, _ := suggestedBaseCoef.Float64()

	latest := uint8(math.Ceil(avgBaseCoefFloat64))
	suggested := uint8(math.Ceil(suggestedBaseCoefFloat64))

	// Safeguard against overflow in uint8
	if latest > 255 {
		latest = 255
	}
	if suggested > 255 {
		suggested = 255
	}

	return utils.WriteJSON(w, &JSONGasBaseCoefPrice{
		Latest:    latest,
		Suggested: suggested,
		// todo remove reason
		Reason: fmt.Sprintf("%s Base Coef - Avg: %s, Sug %s", reason, avgBaseCoef.String(), suggestedBaseCoef.String()),
	})
}

func (b *Blocks) handleGetBlock(w http.ResponseWriter, req *http.Request) error {
	revision, err := utils.ParseRevision(mux.Vars(req)["revision"], false)
	if err != nil {
		return utils.BadRequest(errors.WithMessage(err, "revision"))
	}
	expanded := req.URL.Query().Get("expanded")
	if expanded != "" && expanded != "false" && expanded != "true" {
		return utils.BadRequest(errors.WithMessage(errors.New("should be boolean"), "expanded"))
	}

	summary, err := utils.GetSummary(revision, b.repo, b.bft)
	if err != nil {
		if b.repo.IsNotFound(err) {
			return utils.WriteJSON(w, nil)
		}
		return err
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

	jSummary := buildJSONBlockSummary(summary, isTrunk, isFinalized)
	if expanded == "true" {
		txs, err := b.repo.GetBlockTransactions(summary.Header.ID())
		if err != nil {
			return err
		}
		receipts, err := b.repo.GetBlockReceipts(summary.Header.ID())
		if err != nil {
			return err
		}

		return utils.WriteJSON(w, &JSONExpandedBlock{
			jSummary,
			buildJSONEmbeddedTxs(txs, receipts),
		})
	}

	return utils.WriteJSON(w, &JSONCollapsedBlock{
		jSummary,
		summary.Txs,
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
		Name("blocks_get_block").
		HandlerFunc(utils.WrapHandlerFunc(b.handleGetBlock))
	sub.Path("/{id}/baseGasCoef").
		Methods(http.MethodGet).
		Name("blocks_get_baseGasCoef").
		HandlerFunc(utils.WrapHandlerFunc(b.handleGetGasCoef))
}

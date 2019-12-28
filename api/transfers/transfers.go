// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transfers

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/events"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/logdb"
)

type Transfers struct {
	repo *chain.Repository
	db   *logdb.LogDB
}

func New(repo *chain.Repository, db *logdb.LogDB) *Transfers {
	return &Transfers{
		repo,
		db,
	}
}

//Filter query logs with option
func (t *Transfers) filter(ctx context.Context, filter *TransferFilter) ([]*FilteredTransfer, error) {
	rng, err := events.ConvertRange(t.repo.NewBestChain(), filter.Range)
	if err != nil {
		return nil, err
	}

	transfers, err := t.db.FilterTransfers(ctx, &logdb.TransferFilter{
		CriteriaSet: filter.CriteriaSet,
		Range:       rng,
		Options:     filter.Options,
		Order:       filter.Order,
	})
	if err != nil {
		return nil, err
	}
	tLogs := make([]*FilteredTransfer, len(transfers))
	for i, trans := range transfers {
		tLogs[i] = convertTransfer(trans)
	}
	return tLogs, nil
}

func (t *Transfers) handleFilterTransferLogs(w http.ResponseWriter, req *http.Request) error {
	var filter TransferFilter
	if err := utils.ParseJSON(req.Body, &filter); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	tLogs, err := t.filter(req.Context(), &filter)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, tLogs)
}

func (t *Transfers) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(t.handleFilterTransferLogs))
}

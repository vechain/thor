// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transfers

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/logdb"
)

type Transfers struct {
	db *logdb.LogDB
}

func New(db *logdb.LogDB) *Transfers {
	return &Transfers{
		db,
	}
}

//Filter query logs with option
func (t *Transfers) filter(ctx context.Context, filter *logdb.TransferFilter) ([]*FilteredTransfer, error) {
	transfers, err := t.db.FilterTransfers(ctx, filter)
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
	var filter logdb.TransferFilter
	if err := utils.ParseJSON(req.Body, &filter); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	order := req.URL.Query().Get("order")
	if order != string(logdb.DESC) {
		filter.Order = logdb.ASC
	} else {
		filter.Order = logdb.DESC
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

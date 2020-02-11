// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package events

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/logdb"
)

type Events struct {
	repo *chain.Repository
	db   *logdb.LogDB
}

func New(repo *chain.Repository, db *logdb.LogDB) *Events {
	return &Events{
		repo,
		db,
	}
}

//Filter query events with option
func (e *Events) filter(ctx context.Context, ef *EventFilter) ([]*FilteredEvent, error) {
	chain := e.repo.NewBestChain()
	filter, err := convertEventFilter(chain, ef)
	if err != nil {
		return nil, err
	}
	events, err := e.db.FilterEvents(ctx, filter)
	if err != nil {
		return nil, err
	}
	fes := make([]*FilteredEvent, len(events))
	for i, e := range events {
		fes[i] = convertEvent(e)
	}
	return fes, nil
}

func (e *Events) handleFilter(w http.ResponseWriter, req *http.Request) error {
	var filter EventFilter
	if err := utils.ParseJSON(req.Body, &filter); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	fes, err := e.filter(req.Context(), &filter)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, fes)
}

func (e *Events) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(e.handleFilter))
}

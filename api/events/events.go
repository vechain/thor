// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package events

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
)

type Events struct {
	repo  *chain.Repository
	db    *logdb.LogDB
	limit uint64
}

func New(repo *chain.Repository, db *logdb.LogDB, logsLimit uint64) *Events {
	return &Events{
		repo,
		db,
		logsLimit,
	}
}

// Filter query events with option
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
		fes[i] = convertEvent(e, ef.Options.IncludeIndexes)
	}
	return fes, nil
}

func (e *Events) handleFilter(w http.ResponseWriter, req *http.Request) error {
	var filter EventFilter
	if err := utils.ParseJSON(req.Body, &filter); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	if filter.Options != nil && filter.Options.Limit > e.limit {
		return utils.Forbidden(fmt.Errorf("options.limit exceeds the maximum allowed value of %d", e.limit))
	}
	if filter.Options == nil {
		// if filter.Options is nil, set to the default limit +1
		// to detect whether there are more logs than the default limit
		filter.Options = &Options{
			Offset:         0,
			Limit:          e.limit + 1,
			IncludeIndexes: false,
		}
	}

	fes, err := e.filter(req.Context(), &filter)
	if err != nil {
		return err
	}

	// ensure the result size is less than the configured limit
	if len(fes) > int(e.limit) {
		return utils.Forbidden(fmt.Errorf("the number of filtered logs exceeds the maximum allowed value of %d, please use pagination", e.limit))
	}

	return utils.WriteJSON(w, fes)
}

func (e *Events) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").
		Methods(http.MethodPost).
		Name("logs_filter_event").
		HandlerFunc(utils.WrapHandlerFunc(e.handleFilter))
}

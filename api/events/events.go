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

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/restutil"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logsdb"
)

type Events struct {
	repo  *chain.Repository
	db    logsdb.LogsDB
	limit uint64
}

func New(repo *chain.Repository, db logsdb.LogsDB, logsLimit uint64) *Events {
	return &Events{
		repo,
		db,
		logsLimit,
	}
}

// Filter query events with option
func (e *Events) filter(ctx context.Context, ef *api.EventFilter) ([]*api.FilteredEvent, error) {
	chain := e.repo.NewBestChain()
	filter, err := api.ConvertEventFilter(chain, ef)
	if err != nil {
		return nil, err
	}
	events, err := e.db.FilterEvents(ctx, filter)
	if err != nil {
		return nil, err
	}
	fes := make([]*api.FilteredEvent, len(events))
	for i, e := range events {
		fes[i] = api.ConvertEvent(e, ef.Options.IncludeIndexes)
	}
	return fes, nil
}

func (e *Events) handleFilter(w http.ResponseWriter, req *http.Request) error {
	var filter api.EventFilter
	if err := restutil.ParseJSON(req.Body, &filter); err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "body"))
	}
	if err := filter.Options.Validate(e.limit); err != nil {
		return restutil.Forbidden(err)
	}
	if err := filter.Range.Validate(); err != nil {
		return restutil.BadRequest(err)
	}
	// reject null element in CriteriaSet, {} will be unmarshaled to default value and will be accepted/handled by the filter engine
	for i, criterion := range filter.CriteriaSet {
		if criterion == nil {
			return restutil.BadRequest(fmt.Errorf("criteriaSet[%d]: null not allowed", i))
		}
	}
	if filter.Options == nil {
		filter.Options = &api.Options{}
	}
	if filter.Options.Limit == nil {
		// if filter.Options.Limit is nil, set to the default limit +1
		// to detect whether there are more logs than the default limit
		limit := e.limit + 1
		filter.Options.Limit = &limit
	}

	fes, err := e.filter(req.Context(), &filter)
	if err != nil {
		return err
	}

	// ensure the result size is less than the configured limit
	if len(fes) > int(e.limit) {
		return restutil.Forbidden(fmt.Errorf("the number of filtered logs exceeds the maximum allowed value of %d, please use pagination", e.limit))
	}

	return restutil.WriteJSON(w, fes)
}

func (e *Events) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").
		Methods(http.MethodPost).
		Name("POST /logs/event").
		HandlerFunc(restutil.WrapHandlerFunc(e.handleFilter))
}

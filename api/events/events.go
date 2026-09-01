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
	"github.com/vechain/thor/v2/logdb"
)

type Events struct {
	repo             *chain.Repository
	db               *logdb.LogDB
	maxLimit         uint64
	maxOffset        uint64
	maxCriteriaCount int
}

func New(repo *chain.Repository, db *logdb.LogDB, maxLimit uint64, maxOffset uint64, maxCriteriaCount int) *Events {
	return &Events{
		repo,
		db,
		maxLimit,
		maxOffset,
		maxCriteriaCount,
	}
}

// Filter query events with option. Rows are returned in their logdb form; the
// conversion to the response shape hex-expands Data to roughly twice its size and
// is deferred to the response writer so only one converted event exists at a time.
func (e *Events) filter(ctx context.Context, ef *api.EventFilter) ([]*logdb.Event, error) {
	chain := e.repo.NewBestChain()
	filter, err := api.ConvertEventFilter(chain, ef)
	if err != nil {
		return nil, err
	}
	return e.db.FilterEvents(ctx, filter)
}

func (e *Events) handleFilter(w http.ResponseWriter, req *http.Request) error {
	var filter api.EventFilter
	if err := restutil.ParseJSON(req.Body, &filter); err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "body"))
	}
	if err := filter.Options.Validate(e.maxLimit, e.maxOffset); err != nil {
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
	if len(filter.CriteriaSet) > e.maxCriteriaCount {
		return restutil.BadRequest(fmt.Errorf(
			"number of criteria in criteriaSet: %d cannot be greater than: %d",
			len(filter.CriteriaSet),
			e.maxCriteriaCount),
		)
	}
	if filter.Options == nil {
		filter.Options = &api.Options{}
	}
	if filter.Options.Limit == nil {
		// if filter.Options.Limit is nil, set to the default limit +1
		// to detect whether there are more logs than the default limit
		limit := e.maxLimit + 1
		filter.Options.Limit = &limit
	}

	events, err := e.filter(req.Context(), &filter)
	if err != nil {
		return err
	}

	// ensure the result size is less than the configured limit
	if len(events) > int(e.maxLimit) {
		return restutil.Forbidden(fmt.Errorf("the number of filtered logs exceeds the maximum allowed value of %d, please use pagination", e.maxLimit))
	}

	return restutil.WriteJSONArray(w, len(events), func(i int) *api.FilteredEvent {
		return api.ConvertEvent(events[i], filter.Options.IncludeIndexes)
	})
}

func (e *Events) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").
		Methods(http.MethodPost).
		Name("POST /logs/event").
		HandlerFunc(restutil.WrapHandlerFunc(e.handleFilter))
}

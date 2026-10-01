// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package events

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/convert"
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
	filter, err := convert.ConvertEventFilter(chain, ef)
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
	options, err := restutil.PrepareLogFilter(filter.Options, filter.Range, filter.CriteriaSet, e.maxLimit, e.maxOffset, e.maxCriteriaCount)
	if err != nil {
		return err
	}
	filter.Options = options

	events, err := e.filter(req.Context(), &filter)
	if err != nil {
		return err
	}

	if err := restutil.CheckLogCount(len(events), e.maxLimit); err != nil {
		return err
	}

	return restutil.WriteJSONArray(w, len(events), func(i int) *api.FilteredEvent {
		return convert.ConvertEvent(events[i], filter.Options.IncludeIndexes)
	})
}

func (e *Events) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").
		Methods(http.MethodPost).
		Name("POST /logs/event").
		HandlerFunc(restutil.WrapHandlerFunc(e.handleFilter))
}

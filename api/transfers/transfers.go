// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transfers

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

type Transfers struct {
	repo             *chain.Repository
	db               *logdb.LogDB
	maxLimit         uint64
	maxOffset        uint64
	maxCriteriaCount int
}

func New(repo *chain.Repository, db *logdb.LogDB, maxLimit uint64, maxOffset uint64, maxCriteriaCount int) *Transfers {
	return &Transfers{
		repo,
		db,
		maxLimit,
		maxOffset,
		maxCriteriaCount,
	}
}

// Filter query logs with option. Rows are returned in their logdb form; the
// conversion to the response shape is deferred to the response writer so only one
// converted transfer exists at a time.
func (t *Transfers) filter(ctx context.Context, filter *api.TransferFilter) ([]*logdb.Transfer, error) {
	rng, err := convert.ConvertRange(t.repo.NewBestChain(), filter.Range)
	if err != nil {
		return nil, err
	}

	return t.db.FilterTransfers(ctx, &logdb.TransferFilter{
		CriteriaSet: convert.ConvertTransferCriteria(filter.CriteriaSet),
		Range:       rng,
		Options: &logdb.Options{
			Offset: filter.Options.Offset,
			Limit:  *filter.Options.Limit,
		},
		Order: logdb.Order(filter.Order),
	})
}

func (t *Transfers) handleFilterTransferLogs(w http.ResponseWriter, req *http.Request) error {
	var filter api.TransferFilter
	if err := restutil.ParseJSON(req.Body, &filter); err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "body"))
	}
	options, err := restutil.PrepareLogFilter(filter.Options, filter.Range, filter.CriteriaSet, t.maxLimit, t.maxOffset, t.maxCriteriaCount)
	if err != nil {
		return err
	}
	filter.Options = options

	transfers, err := t.filter(req.Context(), &filter)
	if err != nil {
		return err
	}

	if err := restutil.CheckLogCount(len(transfers), t.maxLimit); err != nil {
		return err
	}

	return restutil.WriteJSONArray(w, len(transfers), func(i int) *api.FilteredTransfer {
		return convert.ConvertTransfer(transfers[i], filter.Options.IncludeIndexes)
	})
}

func (t *Transfers) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").
		Methods(http.MethodPost).
		Name("POST /logs/transfer").
		HandlerFunc(restutil.WrapHandlerFunc(t.handleFilterTransferLogs))
}

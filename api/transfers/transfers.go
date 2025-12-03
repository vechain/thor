// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transfers

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

const MaxCriteriaCount = 10

type Transfers struct {
	repo  *chain.Repository
	db    *logdb.LogDB
	limit uint64
}

func New(repo *chain.Repository, db *logdb.LogDB, logsLimit uint64) *Transfers {
	return &Transfers{
		repo,
		db,
		logsLimit,
	}
}

// Filter query logs with option
func (t *Transfers) filter(ctx context.Context, filter *api.TransferFilter) ([]*api.FilteredTransfer, error) {
	rng, err := api.ConvertRange(t.repo.NewBestChain(), filter.Range)
	if err != nil {
		return nil, err
	}

	transfers, err := t.db.FilterTransfers(ctx, &logdb.TransferFilter{
		CriteriaSet: filter.CriteriaSet,
		Range:       rng,
		Options: &logdb.Options{
			Offset: filter.Options.Offset,
			Limit:  *filter.Options.Limit,
		},
		Order: filter.Order,
	})
	if err != nil {
		return nil, err
	}
	tLogs := make([]*api.FilteredTransfer, len(transfers))
	for i, trans := range transfers {
		tLogs[i] = api.ConvertTransfer(trans, filter.Options.IncludeIndexes)
	}
	return tLogs, nil
}

func (t *Transfers) handleFilterTransferLogs(w http.ResponseWriter, req *http.Request) error {
	var filter api.TransferFilter
	if err := restutil.ParseJSON(req.Body, &filter); err != nil {
		return restutil.BadRequest(errors.WithMessage(err, "body"))
	}
	if err := filter.Options.Validate(t.limit); err != nil {
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
	if len(filter.CriteriaSet) > MaxCriteriaCount {
		return restutil.BadRequest(fmt.Errorf(
			"number of criteria in criteriaSet: %d cannot be greater than: %d",
			len(filter.CriteriaSet),
			MaxCriteriaCount),
		)
	}
	if filter.Options == nil {
		filter.Options = &api.Options{}
	}
	if filter.Options.Limit == nil {
		// if filter.Options.Limit is nil, set to the default limit +1
		// to detect whether there are more logs than the default limit
		limit := t.limit + 1
		filter.Options.Limit = &limit
	}

	tLogs, err := t.filter(req.Context(), &filter)
	if err != nil {
		return err
	}

	// ensure the result size is less than the configured limit
	if len(tLogs) > int(t.limit) {
		return restutil.Forbidden(fmt.Errorf("the number of filtered logs exceeds the maximum allowed value of %d, please use pagination", t.limit))
	}

	return restutil.WriteJSON(w, tLogs)
}

func (t *Transfers) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").
		Methods(http.MethodPost).
		Name("POST /logs/transfer").
		HandlerFunc(restutil.WrapHandlerFunc(t.handleFilterTransferLogs))
}

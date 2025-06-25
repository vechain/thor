// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transfers

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/api/utils"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
)

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
			Limit:  filter.Options.Limit,
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
	if err := utils.ParseJSON(req.Body, &filter); err != nil {
		return utils.BadRequest(errors.WithMessage(err, "body"))
	}
	if filter.Options != nil && filter.Options.Limit > t.limit {
		return utils.Forbidden(fmt.Errorf("options.limit exceeds the maximum allowed value of %d", t.limit))
	}
	if filter.Options != nil && filter.Options.Offset > math.MaxInt64 {
		return utils.BadRequest(fmt.Errorf("options.offset exceeds the maximum allowed value of %d", math.MaxInt64))
	}
	if filter.Range != nil && filter.Range.From != nil && filter.Range.To != nil && *filter.Range.From > *filter.Range.To {
		return utils.BadRequest(fmt.Errorf("filter.Range.To must be greater than or equal to filter.Range.From"))
	}
	// reject null element in CriteriaSet, {} will be unmarshaled to default value and will be accepted/handled by the filter engine
	for i, criterion := range filter.CriteriaSet {
		if criterion == nil {
			return utils.BadRequest(fmt.Errorf("criteriaSet[%d]: null not allowed", i))
		}
	}
	if filter.Options == nil {
		// if filter.Options is nil, set to the default limit +1
		// to detect whether there are more logs than the default limit
		filter.Options = &api.Options{
			Offset:         0,
			Limit:          t.limit + 1,
			IncludeIndexes: false,
		}
	}

	tLogs, err := t.filter(req.Context(), &filter)
	if err != nil {
		return err
	}

	// ensure the result size is less than the configured limit
	if len(tLogs) > int(t.limit) {
		return utils.Forbidden(fmt.Errorf("the number of filtered logs exceeds the maximum allowed value of %d, please use pagination", t.limit))
	}

	return utils.WriteJSON(w, tLogs)
}

func (t *Transfers) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").
		Methods(http.MethodPost).
		Name("POST /logs/transfer").
		HandlerFunc(utils.WrapHandlerFunc(t.handleFilterTransferLogs))
}

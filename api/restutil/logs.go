// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package restutil

import (
	"fmt"

	"github.com/vechain/thor/v2/api"
)

// PrepareLogFilter validates the common parts of a /logs filter request and
// returns the options to query with. Checks run in this order: options (403),
// range (400), null criteria (400), criteria count (400). Missing options
// default to a limit of maxLimit+1, so CheckLogCount can detect overflow.
func PrepareLogFilter[C any](
	options *api.Options,
	rng *api.Range,
	criteriaSet []*C,
	maxLimit, maxOffset uint64,
	maxCriteriaCount int,
) (*api.Options, error) {
	if err := options.Validate(maxLimit, maxOffset); err != nil {
		return nil, Forbidden(err)
	}
	if err := rng.Validate(); err != nil {
		return nil, BadRequest(err)
	}
	// reject null element in CriteriaSet, {} will be unmarshaled to default value and will be accepted/handled by the filter engine
	for i, criterion := range criteriaSet {
		if criterion == nil {
			return nil, BadRequest(fmt.Errorf("criteriaSet[%d]: null not allowed", i))
		}
	}
	if len(criteriaSet) > maxCriteriaCount {
		return nil, BadRequest(fmt.Errorf(
			"number of criteria in criteriaSet: %d cannot be greater than: %d",
			len(criteriaSet),
			maxCriteriaCount),
		)
	}
	if options == nil {
		options = &api.Options{}
	}
	if options.Limit == nil {
		// if filter.Options.Limit is nil, set to the default limit +1
		// to detect whether there are more logs than the default limit
		limit := maxLimit + 1
		options.Limit = &limit
	}
	return options, nil
}

// CheckLogCount rejects (403) a result of more than maxLimit logs.
func CheckLogCount(n int, maxLimit uint64) error {
	if n > int(maxLimit) {
		return Forbidden(fmt.Errorf("the number of filtered logs exceeds the maximum allowed value of %d, please use pagination", maxLimit))
	}
	return nil
}

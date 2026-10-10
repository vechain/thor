// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package events

import (
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/vechain/thor/v2/api/convert"
	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/thor"
)

// ConvertEvent converts a logdb.Event into a json format Event
func ConvertEvent(event *logdb.Event, addIndexes bool) *dto.FilteredEvent {
	fe := &dto.FilteredEvent{
		Address: event.Address,
		Data:    hexutil.Encode(event.Data),
		Meta: dto.LogMeta{
			BlockID:        event.BlockID,
			BlockNumber:    event.BlockNumber,
			BlockTimestamp: event.BlockTime,
			TxID:           event.TxID,
			TxOrigin:       event.TxOrigin,
			ClauseIndex:    event.ClauseIndex,
		},
	}

	if addIndexes {
		fe.Meta.TxIndex = &event.TxIndex
		fe.Meta.LogIndex = &event.LogIndex
	}

	fe.Topics = make([]thor.Bytes32, 0)
	for i := range 5 {
		if event.Topics[i] != nil {
			fe.Topics = append(fe.Topics, *event.Topics[i])
		}
	}
	return fe
}

func ConvertEventFilter(chain *chain.Chain, filter *dto.EventFilter) (*logdb.EventFilter, error) {
	rng, err := convert.ConvertRange(chain, filter.Range)
	if err != nil {
		return nil, err
	}
	criteria := convert.MapSlice(filter.CriteriaSet, func(c *dto.EventCriteria) *logdb.EventCriteria {
		return &logdb.EventCriteria{
			Address: c.Address,
			Topics:  [5]*thor.Bytes32{c.Topic0, c.Topic1, c.Topic2, c.Topic3, c.Topic4},
		}
	})

	return &logdb.EventFilter{
		Range: rng,
		Options: &logdb.Options{
			Offset: filter.Options.Offset,
			// validated or default value set at the API level
			Limit: *filter.Options.Limit,
		},
		Order:       logdb.Order(filter.Order),
		CriteriaSet: criteria,
	}, nil
}

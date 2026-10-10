// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package convert

import (
	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
)

var emptyRange = logdb.Range{
	From: logdb.MaxBlockNumber,
	To:   logdb.MaxBlockNumber,
}

func ConvertRange(chain *chain.Chain, r *dto.Range) (*logdb.Range, error) {
	if r == nil {
		return nil, nil
	}

	if r.Unit == dto.TimeRangeType {
		genesis, err := chain.GetBlockHeader(0)
		if err != nil {
			return nil, err
		}
		if r.To != nil && *r.To < genesis.Timestamp() {
			return &emptyRange, nil
		}
		head, err := chain.GetBlockHeader(block.Number(chain.HeadID()))
		if err != nil {
			return nil, err
		}
		if r.From != nil && *r.From > head.Timestamp() {
			return &emptyRange, nil
		}

		fromHeader := genesis
		if r.From != nil {
			fromHeader, err = chain.FindBlockHeaderByTimestamp(*r.From, 1)
			if err != nil {
				return nil, err
			}
		}

		toHeader := head
		if r.To != nil {
			toHeader, err = chain.FindBlockHeaderByTimestamp(*r.To, -1)
			if err != nil {
				return nil, err
			}
		}

		// A window that falls between two consecutive blocks yields fromBlock > toBlock.
		// logdb drops the upper bound on an inverted range and returns every event from
		// fromBlock onward (results outside the requested window), so collapse it to
		// emptyRange instead.
		if fromHeader.Number() > toHeader.Number() {
			return &emptyRange, nil
		}

		return &logdb.Range{
			From: fromHeader.Number(),
			To:   toHeader.Number(),
		}, nil
	}

	// Units are block numbers - numbers will have a max ceiling at logdb.MaxBlockNumber
	if r.From != nil && *r.From > logdb.MaxBlockNumber {
		return &emptyRange, nil
	}

	from := uint32(0)
	if r.From != nil {
		from = uint32(*r.From)
	}

	to := uint32(logdb.MaxBlockNumber)
	if r.To != nil && *r.To < logdb.MaxBlockNumber {
		to = uint32(*r.To)
	}

	return &logdb.Range{
		From: from,
		To:   to,
	}, nil
}

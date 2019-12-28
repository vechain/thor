// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type eventReader struct {
	repo        *chain.Repository
	filter      *EventFilter
	blockReader chain.BlockReader
}

func newEventReader(repo *chain.Repository, position thor.Bytes32, filter *EventFilter) *eventReader {
	return &eventReader{
		repo:        repo,
		filter:      filter,
		blockReader: repo.NewBlockReader(position),
	}
}

func (er *eventReader) Read() ([]interface{}, bool, error) {
	blocks, err := er.blockReader.Read()
	if err != nil {
		return nil, false, err
	}
	var msgs []interface{}
	for _, block := range blocks {
		receipts, err := er.repo.GetBlockReceipts(block.Header().ID())
		if err != nil {
			return nil, false, err
		}
		txs := block.Transactions()
		for i, receipt := range receipts {
			for j, output := range receipt.Outputs {
				for _, event := range output.Events {
					if er.filter.Match(event) {
						msg, err := convertEvent(block.Header(), txs[i], uint32(j), event, block.Obsolete)
						if err != nil {
							return nil, false, err
						}
						msgs = append(msgs, msg)
					}
				}
			}
		}
	}
	return msgs, len(blocks) > 0, nil
}

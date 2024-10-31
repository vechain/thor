// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"encoding/json"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

type transferReader struct {
	repo        *chain.Repository
	filter      *TransferFilter
	blockReader chain.BlockReader
}

func newTransferReader(repo *chain.Repository, position thor.Bytes32, filter *TransferFilter) *transferReader {
	return &transferReader{
		repo:        repo,
		filter:      filter,
		blockReader: repo.NewBlockReader(position),
	}
}

func (tr *transferReader) Read() ([]rawMessage, bool, error) {
	blocks, err := tr.blockReader.Read()
	if err != nil {
		return nil, false, err
	}
	var msgs []rawMessage
	for _, block := range blocks {
		receipts, err := tr.repo.GetBlockReceipts(block.Header().ID())
		if err != nil {
			return nil, false, err
		}
		txs := block.Transactions()
		for i, receipt := range receipts {
			for j, output := range receipt.Outputs {
				for _, transfer := range output.Transfers {
					origin, err := txs[i].Origin()
					if err != nil {
						return nil, false, err
					}
					if tr.filter.Match(transfer, origin) {
						msg, err := convertTransfer(block.Header(), txs[i], uint32(j), transfer, block.Obsolete)
						if err != nil {
							return nil, false, err
						}
						bytes, err := json.Marshal(msg)
						if err != nil {
							return nil, false, err
						}
						msgs = append(msgs, bytes)
					}
				}
			}
		}
	}
	return msgs, len(blocks) > 0, nil
}

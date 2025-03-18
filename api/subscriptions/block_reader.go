// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

type blockReader struct {
	blockReader chain.BlockReader
}

func newBlockReader(repo *chain.Repository, position thor.Bytes32) *blockReader {
	return &blockReader{
		blockReader: repo.NewBlockReader(position),
	}
}

func (br *blockReader) Read() ([]any, bool, error) {
	blocks, err := br.blockReader.Read()
	if err != nil {
		return nil, false, err
	}
	var msgs []any
	for _, block := range blocks {
		msg, err := convertBlock(block)
		if err != nil {
			return nil, false, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, len(blocks) > 0, nil
}

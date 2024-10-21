// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"encoding/json"

	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
)

type blockReader struct {
	blockReader chain.BlockReader
	cache       *messageCache
}

func newBlockReader(repo *chain.Repository, position thor.Bytes32, cache *messageCache) *blockReader {
	return &blockReader{
		blockReader: repo.NewBlockReader(position),
		cache:       cache,
	}
}

func (br *blockReader) Read() ([][]byte, bool, error) {
	blocks, err := br.blockReader.Read()
	if err != nil {
		return nil, false, err
	}
	var msgs [][]byte
	for _, block := range blocks {
		msg, _, err := br.cache.GetOrAdd(block.Header().ID(), br.generateBlockMessage(block))
		if err != nil {
			return nil, false, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, len(blocks) > 0, nil
}

func (br *blockReader) generateBlockMessage(block *chain.ExtendedBlock) func() ([]byte, error) {
	return func() ([]byte, error) {
		blk, err := convertBlock(block)
		if err != nil {
			return nil, err
		}
		return json.Marshal(blk)
	}
}

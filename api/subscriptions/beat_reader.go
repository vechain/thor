// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thor/bloom"
)

type beatReader struct {
	repo        *chain.Repository
	blockReader chain.BlockReader
	cache       *messageCache[BeatMessage]
}

func newBeatReader(repo *chain.Repository, position thor.Bytes32, cache *messageCache[BeatMessage]) *beatReader {
	return &beatReader{
		repo:        repo,
		blockReader: repo.NewBlockReader(position),
		cache:       cache,
	}
}

func (br *beatReader) Read() ([]interface{}, bool, error) {
	blocks, err := br.blockReader.Read()
	if err != nil {
		return nil, false, err
	}
	var msgs []interface{}
	for _, block := range blocks {
		msg, _, err := br.cache.GetOrAdd(block.Header().ID(), br.generateBeatMessage(block))
		if err != nil {
			return nil, false, err
		}
		msgs = append(msgs, msg)
	}
	return msgs, len(blocks) > 0, nil
}

func (br *beatReader) generateBeatMessage(block *chain.ExtendedBlock) func() (BeatMessage, error) {
	return func() (BeatMessage, error) {
		header := block.Header()
		receipts, err := br.repo.GetBlockReceipts(header.ID())
		if err != nil {
			return BeatMessage{}, err
		}
		txs := block.Transactions()
		content := &bloomContent{}
		for i, receipt := range receipts {
			content.add(receipt.GasPayer.Bytes())
			for _, output := range receipt.Outputs {
				for _, event := range output.Events {
					content.add(event.Address.Bytes())
					for _, topic := range event.Topics {
						content.add(topic.Bytes())
					}
				}
				for _, transfer := range output.Transfers {
					content.add(transfer.Sender.Bytes())
					content.add(transfer.Recipient.Bytes())
				}
			}
			origin, _ := txs[i].Origin()
			content.add(origin.Bytes())
		}
		signer, _ := header.Signer()
		content.add(signer.Bytes())
		content.add(header.Beneficiary().Bytes())

		k := bloom.LegacyEstimateBloomK(content.len())
		bloom := bloom.NewLegacyBloom(k)
		for _, item := range content.items {
			bloom.Add(item)
		}
		beat := BeatMessage{
			Number:      header.Number(),
			ID:          header.ID(),
			ParentID:    header.ParentID(),
			Timestamp:   header.Timestamp(),
			TxsFeatures: uint32(header.TxsFeatures()),
			Bloom:       hexutil.Encode(bloom.Bits[:]),
			K:           uint32(k),
			Obsolete:    block.Obsolete,
		}

		return beat, nil
	}
}

type bloomContent struct {
	items [][]byte
}

func (bc *bloomContent) add(item []byte) {
	bc.items = append(bc.items, bytes.TrimLeft(item, "\x00"))
}

func (bc *bloomContent) len() int {
	return len(bc.items)
}

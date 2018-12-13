// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type beatReader struct {
	chain       *chain.Chain
	blockReader chain.BlockReader
}

func newBeatReader(chain *chain.Chain, position thor.Bytes32) *beatReader {
	return &beatReader{
		chain:       chain,
		blockReader: chain.NewBlockReader(position),
	}
}

func (br *beatReader) Read() ([]interface{}, bool, error) {
	blocks, err := br.blockReader.Read()
	if err != nil {
		return nil, false, err
	}
	var msgs []interface{}
	for _, block := range blocks {
		header := block.Header()
		receipts, err := br.chain.GetBlockReceipts(header.ID())
		if err != nil {
			return nil, false, err
		}
		txs := block.Transactions()
		bloomContent := &bloomContent{}
		for i, receipt := range receipts {
			for _, output := range receipt.Outputs {
				for _, event := range output.Events {
					bloomContent.add(event.Address.Bytes())
					for _, topic := range event.Topics {
						bloomContent.add(topic.Bytes())
					}
				}
				for _, transfer := range output.Transfers {
					bloomContent.add(transfer.Sender.Bytes())
					bloomContent.add(transfer.Recipient.Bytes())
				}
			}
			origin, _ := txs[i].Signer()
			bloomContent.add(origin.Bytes())
		}
		signer, _ := header.Signer()
		bloomContent.add(signer.Bytes())
		bloomContent.add(header.Beneficiary().Bytes())

		k := thor.EstimateBloomK(bloomContent.len())
		bloom := thor.NewBloom(k)
		for _, item := range bloomContent.items {
			bloom.Add(item)
		}
		msgs = append(msgs, &BeatMessage{
			Number:    header.Number(),
			ID:        header.ID(),
			ParentID:  header.ParentID(),
			Timestamp: header.Timestamp(),
			Bloom:     hexutil.Encode(bloom.Bits[:]),
			K:         uint32(k),
			Obsolete:  block.Obsolete,
		})
	}
	return msgs, len(blocks) > 0, nil
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

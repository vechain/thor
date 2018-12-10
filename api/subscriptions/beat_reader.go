// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
)

type beatReader struct {
	chain       *chain.Chain
	blockReader chain.BlockReader
}

func newBitsReader(chain *chain.Chain, position thor.Bytes32) *beatReader {
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
		var bloom types.Bloom
		txs := block.Transactions()
		for i, receipt := range receipts {
			for _, output := range receipt.Outputs {
				for _, event := range output.Events {
					bloom = appendData(bloom, event.Address.Bytes())
					for _, topic := range event.Topics {
						bloom.Add(new(big.Int).SetBytes(topic.Bytes()))
					}
				}
				for _, transfer := range output.Transfers {
					bloom = appendData(bloom, transfer.Sender.Bytes())
					bloom = appendData(bloom, transfer.Recipient.Bytes())
				}
			}
			origin, _ := txs[i].Signer()
			bloom = appendData(bloom, origin.Bytes())
		}
		signer, _ := header.Signer()
		bloom = appendData(bloom, signer.Bytes())
		bloom = appendData(bloom, header.Beneficiary().Bytes())
		msgs = append(msgs, &BeatMessage{
			Number:    header.Number(),
			ID:        header.ID(),
			ParentID:  header.ParentID(),
			Timestamp: header.Timestamp(),
			Bloom:     hexutil.Encode(bloom.Bytes()),
		})
	}
	return msgs, len(blocks) > 0, nil
}

func appendData(bloom types.Bloom, data []byte) types.Bloom {
	bloom.Add(new(big.Int).SetBytes(data))
	return bloom
}

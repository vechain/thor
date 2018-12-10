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

type bitsReader struct {
	chain       *chain.Chain
	blockReader chain.BlockReader
}

func newBitsReader(chain *chain.Chain, position thor.Bytes32) *bitsReader {
	return &bitsReader{
		chain:       chain,
		blockReader: chain.NewBlockReader(position),
	}
}

func (br *bitsReader) Read() ([]interface{}, bool, error) {
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
		var bitMsgs []string
		var bloom types.Bloom
		txs := block.Transactions()
		for i, receipt := range receipts {
			for _, output := range receipt.Outputs {
				for _, event := range output.Events {
					bitMsgs, bloom = appendData(bitMsgs, bloom, event.Address.Bytes())
					for _, topic := range event.Topics {
						bitMsgs, bloom = appendData(bitMsgs, bloom, topic.Bytes())
					}
				}
				for _, transfer := range output.Transfers {
					bitMsgs, bloom = appendData(bitMsgs, bloom, transfer.Sender.Bytes())
					bitMsgs, bloom = appendData(bitMsgs, bloom, transfer.Sender.Bytes())
				}
			}
			origin, _ := txs[i].Signer()
			bitMsgs, bloom = appendData(bitMsgs, bloom, origin.Bytes())
		}
		signer, _ := header.Signer()
		bitMsgs, bloom = appendData(bitMsgs, bloom, signer.Bytes())
		bitMsgs, bloom = appendData(bitMsgs, bloom, header.Beneficiary().Bytes())
		blockMsg, err := convertBlock(block)
		if err != nil {
			return nil, false, err
		}
		msgs = append(msgs, &BloomMessage{
			Block: blockMsg,
			Bits:  bitMsgs,
			Bloom: hexutil.Encode(bloom.Bytes()),
		})
	}
	return msgs, len(blocks) > 0, nil
}

func appendData(bitMsgs []string, bloom types.Bloom, data []byte) ([]string, types.Bloom) {
	bitMsgs = append(bitMsgs, hexutil.Encode(data))
	bloom.Add(new(big.Int).SetBytes(data))
	return bitMsgs, bloom
}

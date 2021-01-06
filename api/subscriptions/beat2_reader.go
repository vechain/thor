// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package subscriptions

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/thor/bloom"
)

type beat2Reader struct {
	repo        *chain.Repository
	blockReader chain.BlockReader
}

func newBeat2Reader(repo *chain.Repository, position thor.Bytes32) *beat2Reader {
	return &beat2Reader{
		repo:        repo,
		blockReader: repo.NewBlockReader(position),
	}
}

func (br *beat2Reader) Read() ([]interface{}, bool, error) {
	blocks, err := br.blockReader.Read()
	if err != nil {
		return nil, false, err
	}
	var msgs []interface{}

	bloomGenerator := &bloom.Generator{}

	bloomAdd := func(key []byte) {
		key = bytes.TrimLeft(key, "\x00")
		// exclude non-address key
		if len(key) <= thor.AddressLength {
			bloomGenerator.Add(key)
		}
	}

	for _, block := range blocks {
		header := block.Header()
		receipts, err := br.repo.GetBlockReceipts(header.ID())
		if err != nil {
			return nil, false, err
		}
		txs := block.Transactions()
		for i, receipt := range receipts {
			bloomAdd(receipt.GasPayer.Bytes())
			for _, output := range receipt.Outputs {
				for _, event := range output.Events {
					bloomAdd(event.Address.Bytes())
					for _, topic := range event.Topics {
						bloomAdd(topic.Bytes())
					}
				}
				for _, transfer := range output.Transfers {
					bloomAdd(transfer.Sender.Bytes())
					bloomAdd(transfer.Recipient.Bytes())
				}
			}
			origin, _ := txs[i].Origin()
			bloomAdd(origin.Bytes())
		}
		signer, _ := header.Signer()
		bloomAdd(signer.Bytes())
		bloomAdd(header.Beneficiary().Bytes())

		const bitsPerKey = 20
		filter := bloomGenerator.Generate(bitsPerKey, bloom.K(bitsPerKey))

		msgs = append(msgs, &Beat2Message{
			Number:      header.Number(),
			ID:          header.ID(),
			ParentID:    header.ParentID(),
			Timestamp:   header.Timestamp(),
			TxsFeatures: uint32(header.TxsFeatures()),
			GasLimit:    header.GasLimit(),
			Bloom:       hexutil.Encode(filter.Bits),
			K:           filter.K,
			Obsolete:    block.Obsolete,
		})
	}
	return msgs, len(blocks) > 0, nil
}

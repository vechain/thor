// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api/convert"
	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func ConvertBlockSummary(summary *chain.BlockSummary, isTrunk bool, isFinalized bool) *dto.BlockSummary {
	header := summary.Header
	signer, _ := header.Signer()

	return &dto.BlockSummary{
		BlockBase:   convert.ConvertBlockBase(header, uint32(summary.Size), signer),
		IsTrunk:     isTrunk,
		IsFinalized: isFinalized,
	}
}

func convertOutput(txID thor.Bytes32, index uint32, c *tx.Clause, o *tx.Output) *dto.Output {
	jo := &dto.Output{
		ContractAddress: nil,
		Events:          make([]*dto.Event, 0, len(o.Events)),
		Transfers:       make([]*dto.Transfer, 0, len(o.Transfers)),
	}
	if c.To() == nil {
		addr := thor.CreateContractAddress(txID, index, 0)
		jo.ContractAddress = &addr
	}
	for _, e := range o.Events {
		jo.Events = append(jo.Events, &dto.Event{
			Address: e.Address,
			Data:    hexutil.Encode(e.Data),
			Topics:  e.Topics,
		})
	}
	for _, t := range o.Transfers {
		jo.Transfers = append(jo.Transfers, &dto.Transfer{
			Sender:    t.Sender,
			Recipient: t.Recipient,
			Amount:    (*math.HexOrDecimal256)(t.Amount),
		})
	}
	return jo
}

func ConvertEmbeddedTxs(txs tx.Transactions, receipts tx.Receipts) []*dto.EmbeddedTx {
	jTxs := make([]*dto.EmbeddedTx, 0, len(txs))
	for itx, trx := range txs {
		receipt := receipts[itx]
		clauses := trx.Clauses()

		jos := make([]*dto.Output, 0, len(receipt.Outputs))
		if !receipt.Reverted {
			for i, c := range clauses {
				jos = append(jos, convertOutput(trx.ID(), uint32(i), c, receipt.Outputs[i]))
			}
		}

		embedTx := &dto.EmbeddedTx{
			TransactionBase: convert.ConvertTransactionBase(trx),

			GasUsed:  receipt.GasUsed,
			GasPayer: receipt.GasPayer,
			Paid:     (*math.HexOrDecimal256)(receipt.Paid),
			Reward:   (*math.HexOrDecimal256)(receipt.Reward),
			Reverted: receipt.Reverted,
			Outputs:  jos,
		}
		jTxs = append(jTxs, embedTx)
	}
	return jTxs
}

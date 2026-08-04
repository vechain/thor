// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package blocks

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func ConvertBlockSummary(summary *chain.BlockSummary, isTrunk bool, isFinalized bool) *dto.BlockSummary {
	header := summary.Header
	signer, _ := header.Signer()

	return &dto.BlockSummary{
		Number:        header.Number(),
		ID:            header.ID(),
		ParentID:      header.ParentID(),
		Timestamp:     header.Timestamp(),
		TotalScore:    header.TotalScore(),
		GasLimit:      header.GasLimit(),
		GasUsed:       header.GasUsed(),
		Beneficiary:   header.Beneficiary(),
		Signer:        signer,
		Size:          uint32(summary.Size),
		StateRoot:     header.StateRoot(),
		ReceiptsRoot:  header.ReceiptsRoot(),
		TxsRoot:       header.TxsRoot(),
		TxsFeatures:   uint32(header.TxsFeatures()),
		COM:           header.COM(),
		IsTrunk:       isTrunk,
		IsFinalized:   isFinalized,
		BaseFeePerGas: (*math.HexOrDecimal256)(summary.Header.BaseFee()),
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
		blockRef := trx.BlockRef()
		origin, _ := trx.Origin()
		delegator, _ := trx.Delegator()

		jcs := make([]*dto.Clause, 0, len(clauses))
		jos := make([]*dto.Output, 0, len(receipt.Outputs))

		for i, c := range clauses {
			jcs = append(jcs, &dto.Clause{
				To:    c.To(),
				Value: (*math.HexOrDecimal256)(c.Value()),
				Data:  hexutil.Encode(c.Data()),
			})
			if !receipt.Reverted {
				jos = append(jos, convertOutput(trx.ID(), uint32(i), c, receipt.Outputs[i]))
			}
		}

		embedTx := &dto.EmbeddedTx{
			ID:         trx.ID(),
			Type:       trx.Type(),
			ChainTag:   trx.ChainTag(),
			BlockRef:   hexutil.Encode(blockRef[:]),
			Expiration: trx.Expiration(),
			Clauses:    jcs,
			Gas:        trx.Gas(),
			Origin:     origin,
			Delegator:  delegator,
			Nonce:      math.HexOrDecimal64(trx.Nonce()),
			DependsOn:  trx.DependsOn(),
			Size:       uint32(trx.Size()),

			GasUsed:  receipt.GasUsed,
			GasPayer: receipt.GasPayer,
			Paid:     (*math.HexOrDecimal256)(receipt.Paid),
			Reward:   (*math.HexOrDecimal256)(receipt.Reward),
			Reverted: receipt.Reverted,
			Outputs:  jos,
		}
		if trx.Type() == tx.TypeLegacy {
			coef := trx.GasPriceCoef()
			embedTx.GasPriceCoef = &coef
		} else {
			embedTx.MaxFeePerGas = (*math.HexOrDecimal256)(trx.MaxFeePerGas())
			embedTx.MaxPriorityFeePerGas = (*math.HexOrDecimal256)(trx.MaxPriorityFeePerGas())
		}
		jTxs = append(jTxs, embedTx)
	}
	return jTxs
}

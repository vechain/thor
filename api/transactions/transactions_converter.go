// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api/convert"
	"github.com/vechain/thor/v2/api/dto"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// ConvertTransaction convert a raw transaction into a json format transaction
func ConvertTransaction(trx *tx.Transaction, header *block.Header) *dto.Transaction {
	t := &dto.Transaction{
		TransactionBase: convert.ConvertTransactionBase(trx),
	}

	if header != nil {
		t.Meta = &dto.TxMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
		}
	}
	return t
}

// ConvertReceipt convert a raw clause into a jason format clause
func ConvertReceipt(txReceipt *tx.Receipt, header *block.Header, tx *tx.Transaction) (*dto.Receipt, error) {
	reward := math.HexOrDecimal256(*txReceipt.Reward)
	paid := math.HexOrDecimal256(*txReceipt.Paid)
	origin, err := tx.Origin()
	if err != nil {
		return nil, err
	}
	receipt := &dto.Receipt{
		Type:     txReceipt.Type,
		GasUsed:  txReceipt.GasUsed,
		GasPayer: txReceipt.GasPayer,
		Paid:     &paid,
		Reward:   &reward,
		Reverted: txReceipt.Reverted,
		Meta: dto.ReceiptMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
			TxID:           tx.ID(),
			TxOrigin:       origin,
		},
	}
	txClauses := tx.Clauses()
	receipt.Outputs = make([]*dto.Output, len(txReceipt.Outputs))
	for i, output := range txReceipt.Outputs {
		clause := txClauses[i]
		var contractAddr *thor.Address
		if clause.To() == nil {
			cAddr := thor.CreateContractAddress(tx.ID(), uint32(i), 0)
			contractAddr = &cAddr
		}
		otp := &dto.Output{
			ContractAddress: contractAddr,
			Events:          make([]*dto.Event, len(output.Events)),
			Transfers:       make([]*dto.Transfer, len(output.Transfers)),
		}
		for j, txEvent := range output.Events {
			event := &dto.Event{
				Address: txEvent.Address,
				Data:    hexutil.Encode(txEvent.Data),
			}
			event.Topics = make([]thor.Bytes32, len(txEvent.Topics))
			copy(event.Topics, txEvent.Topics)
			otp.Events[j] = event
		}
		for j, txTransfer := range output.Transfers {
			transfer := &dto.Transfer{
				Sender:    txTransfer.Sender,
				Recipient: txTransfer.Recipient,
				Amount:    (*math.HexOrDecimal256)(txTransfer.Amount),
			}
			otp.Transfers[j] = transfer
		}
		receipt.Outputs[i] = otp
	}
	return receipt, nil
}

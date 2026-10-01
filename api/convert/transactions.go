// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package convert

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// ConvertReceipt convert a raw clause into a jason format clause
func ConvertReceipt(txReceipt *tx.Receipt, header *block.Header, tx *tx.Transaction) (*api.Receipt, error) {
	reward := math.HexOrDecimal256(*txReceipt.Reward)
	paid := math.HexOrDecimal256(*txReceipt.Paid)
	origin, err := tx.Origin()
	if err != nil {
		return nil, err
	}
	receipt := &api.Receipt{
		Type:     txReceipt.Type,
		GasUsed:  txReceipt.GasUsed,
		GasPayer: txReceipt.GasPayer,
		Paid:     &paid,
		Reward:   &reward,
		Reverted: txReceipt.Reverted,
		Meta: api.ReceiptMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
			TxID:           tx.ID(),
			TxOrigin:       origin,
		},
	}
	txClauses := tx.Clauses()
	receipt.Outputs = make([]*api.Output, len(txReceipt.Outputs))
	for i, output := range txReceipt.Outputs {
		clause := txClauses[i]
		var contractAddr *thor.Address
		if clause.To() == nil {
			cAddr := thor.CreateContractAddress(tx.ID(), uint32(i), 0)
			contractAddr = &cAddr
		}
		otp := &api.Output{
			ContractAddress: contractAddr,
			Events:          make([]*api.Event, len(output.Events)),
			Transfers:       make([]*api.Transfer, len(output.Transfers)),
		}
		for j, txEvent := range output.Events {
			event := &api.Event{
				Address: txEvent.Address,
				Data:    hexutil.Encode(txEvent.Data),
			}
			event.Topics = make([]thor.Bytes32, len(txEvent.Topics))
			copy(event.Topics, txEvent.Topics)
			otp.Events[j] = event
		}
		for j, txTransfer := range output.Transfers {
			transfer := &api.Transfer{
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

// ConvertTransaction convert a raw transaction into a json format transaction
func ConvertTransaction(trx *tx.Transaction, header *block.Header) *api.Transaction {
	// tx origin
	origin, _ := trx.Origin()
	delegator, _ := trx.Delegator()

	cls := make(api.Clauses, len(trx.Clauses()))
	for i, c := range trx.Clauses() {
		clause := ConvertClause(c)
		cls[i] = &clause
	}
	br := trx.BlockRef()
	t := &api.Transaction{
		ChainTag:   trx.ChainTag(),
		Type:       trx.Type(),
		ID:         trx.ID(),
		Origin:     origin,
		BlockRef:   hexutil.Encode(br[:]),
		Expiration: trx.Expiration(),
		Nonce:      math.HexOrDecimal64(trx.Nonce()),
		Size:       uint32(trx.Size()),
		Gas:        trx.Gas(),
		DependsOn:  trx.DependsOn(),
		Clauses:    cls,
		Delegator:  delegator,
	}

	switch trx.Type() {
	case tx.TypeLegacy:
		coef := trx.GasPriceCoef()
		t.GasPriceCoef = &coef
	default:
		t.MaxFeePerGas = (*math.HexOrDecimal256)(trx.MaxFeePerGas())
		t.MaxPriorityFeePerGas = (*math.HexOrDecimal256)(trx.MaxPriorityFeePerGas())
	}

	if header != nil {
		t.Meta = &api.TxMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
		}
	}
	return t
}

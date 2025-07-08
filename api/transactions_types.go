// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// ConvertClause convert a raw clause into a json format clause
func ConvertClause(c *tx.Clause) Clause {
	return Clause{
		To:    c.To(),
		Value: (*math.HexOrDecimal256)(c.Value()),
		Data:  hexutil.Encode(c.Data()),
	}
}

func (c *Clause) String() string {
	return fmt.Sprintf(`Clause(
		To    %v
		Value %v
		Data  %v
		)`, c.To,
		c.Value,
		c.Data)
}

type RawTx struct {
	Raw string `json:"raw"`
}

func (rtx *RawTx) Decode() (*tx.Transaction, error) {
	data, err := hexutil.Decode(rtx.Raw)
	if err != nil {
		return nil, err
	}

	tx := new(tx.Transaction)
	if err := tx.UnmarshalBinary(data); err != nil {
		return nil, err
	}
	return tx, nil
}

type RawTransaction struct {
	RawTx
	Meta *TxMeta `json:"meta"`
}

type TxMeta struct {
	BlockID        thor.Bytes32 `json:"blockID"`
	BlockNumber    uint32       `json:"blockNumber"`
	BlockTimestamp uint64       `json:"blockTimestamp"`
}

type ReceiptMeta struct {
	BlockID        thor.Bytes32 `json:"blockID"`
	BlockNumber    uint32       `json:"blockNumber"`
	BlockTimestamp uint64       `json:"blockTimestamp"`
	TxID           thor.Bytes32 `json:"txID"`
	TxOrigin       thor.Address `json:"txOrigin"`
}

// Receipt for json marshal
type Receipt struct {
	Type     uint8                 `json:"type,omitempty"`
	GasUsed  uint64                `json:"gasUsed"`
	GasPayer thor.Address          `json:"gasPayer"`
	Paid     *math.HexOrDecimal256 `json:"paid"`
	Reward   *math.HexOrDecimal256 `json:"reward"`
	Reverted bool                  `json:"reverted"`
	Meta     ReceiptMeta           `json:"meta"`
	Outputs  []*Output             `json:"outputs"`
}

// Output output of clause execution.
type Output struct {
	ContractAddress *thor.Address `json:"contractAddress"`
	Events          []*Event      `json:"events"`
	Transfers       []*Transfer   `json:"transfers"`
}

// Event event.
type Event struct {
	Address thor.Address   `json:"address"`
	Topics  []thor.Bytes32 `json:"topics"`
	Data    string         `json:"data"`
}

// Transfer transfer log.
type Transfer struct {
	Sender    thor.Address          `json:"sender"`
	Recipient thor.Address          `json:"recipient"`
	Amount    *math.HexOrDecimal256 `json:"amount"`
}

// ConvertReceipt convert a raw clause into a jason format clause
func ConvertReceipt(txReceipt *tx.Receipt, header *block.Header, tx *tx.Transaction) (*Receipt, error) {
	reward := math.HexOrDecimal256(*txReceipt.Reward)
	paid := math.HexOrDecimal256(*txReceipt.Paid)
	origin, err := tx.Origin()
	if err != nil {
		return nil, err
	}
	receipt := &Receipt{
		Type:     txReceipt.Type,
		GasUsed:  txReceipt.GasUsed,
		GasPayer: txReceipt.GasPayer,
		Paid:     &paid,
		Reward:   &reward,
		Reverted: txReceipt.Reverted,
		Meta: ReceiptMeta{
			header.ID(),
			header.Number(),
			header.Timestamp(),
			tx.ID(),
			origin,
		},
	}
	txClauses := tx.Clauses()
	receipt.Outputs = make([]*Output, len(txReceipt.Outputs))
	for i, output := range txReceipt.Outputs {
		clause := txClauses[i]
		var contractAddr *thor.Address
		if clause.To() == nil {
			cAddr := thor.CreateContractAddress(tx.ID(), uint32(i), 0)
			contractAddr = &cAddr
		}
		otp := &Output{contractAddr,
			make([]*Event, len(output.Events)),
			make([]*Transfer, len(output.Transfers)),
		}
		for j, txEvent := range output.Events {
			event := &Event{
				Address: txEvent.Address,
				Data:    hexutil.Encode(txEvent.Data),
			}
			event.Topics = make([]thor.Bytes32, len(txEvent.Topics))
			copy(event.Topics, txEvent.Topics)
			otp.Events[j] = event
		}
		for j, txTransfer := range output.Transfers {
			transfer := &Transfer{
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

// SendTxResult is the response to the Send Tx method
type SendTxResult struct {
	ID *thor.Bytes32 `json:"id"`
}

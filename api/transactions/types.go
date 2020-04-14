// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Clause for json marshal
type Clause struct {
	To    *thor.Address        `json:"to"`
	Value math.HexOrDecimal256 `json:"value"`
	Data  string               `json:"data"`
}

//Clauses array of clauses.
type Clauses []Clause

//ConvertClause convert a raw clause into a json format clause
func convertClause(c *tx.Clause) Clause {
	return Clause{
		c.To(),
		math.HexOrDecimal256(*c.Value()),
		hexutil.Encode(c.Data()),
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

//Transaction transaction
type Transaction struct {
	ID           thor.Bytes32        `json:"id"`
	ChainTag     byte                `json:"chainTag"`
	BlockRef     string              `json:"blockRef"`
	Expiration   uint32              `json:"expiration"`
	Clauses      Clauses             `json:"clauses"`
	GasPriceCoef uint8               `json:"gasPriceCoef"`
	Gas          uint64              `json:"gas"`
	Origin       thor.Address        `json:"origin"`
	Delegator    *thor.Address       `json:"delegator"`
	Nonce        math.HexOrDecimal64 `json:"nonce"`
	DependsOn    *thor.Bytes32       `json:"dependsOn"`
	Size         uint32              `json:"size"`
	Meta         *TxMeta             `json:"meta"`
}

type RawTx struct {
	Raw string `json:"raw"`
}

func (rtx *RawTx) decode() (*tx.Transaction, error) {
	data, err := hexutil.Decode(rtx.Raw)
	if err != nil {
		return nil, err
	}
	var tx *tx.Transaction
	if err := rlp.DecodeBytes(data, &tx); err != nil {
		return nil, err
	}
	return tx, nil
}

type rawTransaction struct {
	RawTx
	Meta *TxMeta `json:"meta"`
}

//convertTransaction convert a raw transaction into a json format transaction
func convertTransaction(tx *tx.Transaction, header *block.Header) *Transaction {
	//tx origin
	origin, _ := tx.Origin()
	delegator, _ := tx.Delegator()

	cls := make(Clauses, len(tx.Clauses()))
	for i, c := range tx.Clauses() {
		cls[i] = convertClause(c)
	}
	br := tx.BlockRef()
	t := &Transaction{
		ChainTag:     tx.ChainTag(),
		ID:           tx.ID(),
		Origin:       origin,
		BlockRef:     hexutil.Encode(br[:]),
		Expiration:   tx.Expiration(),
		Nonce:        math.HexOrDecimal64(tx.Nonce()),
		Size:         uint32(tx.Size()),
		GasPriceCoef: tx.GasPriceCoef(),
		Gas:          tx.Gas(),
		DependsOn:    tx.DependsOn(),
		Clauses:      cls,
		Delegator:    delegator,
	}

	if header != nil {
		t.Meta = &TxMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
		}
	}
	return t
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

//Receipt for json marshal
type Receipt struct {
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

//ConvertReceipt convert a raw clause into a jason format clause
func convertReceipt(txReceipt *tx.Receipt, header *block.Header, tx *tx.Transaction) (*Receipt, error) {
	reward := math.HexOrDecimal256(*txReceipt.Reward)
	paid := math.HexOrDecimal256(*txReceipt.Paid)
	origin, err := tx.Origin()
	if err != nil {
		return nil, err
	}
	receipt := &Receipt{
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
	receipt.Outputs = make([]*Output, len(txReceipt.Outputs))
	for i, output := range txReceipt.Outputs {
		clause := tx.Clauses()[i]
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
			for k, topic := range txEvent.Topics {
				event.Topics[k] = topic
			}
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

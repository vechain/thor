// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Clause for json marshal
type Clause struct {
	To    *thor.Address        `json:"to"`
	Value math.HexOrDecimal256 `json:"value"`
	Data  string               `json:"data"`
}

// Clauses array of clauses.
type Clauses []Clause

// ConvertClause convert a raw clause into a json format clause
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

// Transaction transaction
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

type RawTransaction struct {
	RawTx
	Meta *TxMeta `json:"meta"`
}

// ConvertCoreTransaction converts a core type transaction into an api tx (json format transaction)
// allows to specify the origin and delegator
func ConvertCoreTransaction(tx *tx.Transaction, header *block.Header, origin *thor.Address, delegator *thor.Address) *Transaction {
	cls := make(Clauses, len(tx.Clauses()))
	for i, c := range tx.Clauses() {
		cls[i] = convertClause(c)
	}
	br := tx.BlockRef()
	t := &Transaction{
		ChainTag:     tx.ChainTag(),
		ID:           tx.ID(),
		Origin:       *origin,
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

// convertSignedCoreTransaction converts a core type transaction into an api tx (json format transaction)
// retrieves the origin and delegator from signature
func convertTransaction(tx *tx.Transaction, header *block.Header) *Transaction {
	//tx origin
	origin, _ := tx.Origin()
	delegator, _ := tx.Delegator()
	return ConvertCoreTransaction(tx, header, &origin, delegator)
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
	GasUsed  uint64                `json:"gasUsed"`
	GasPayer thor.Address          `json:"gasPayer"`
	Paid     *math.HexOrDecimal256 `json:"paid"`
	Reward   *math.HexOrDecimal256 `json:"reward"`
	Reverted bool                  `json:"reverted"`
	Meta     ReceiptMeta           `json:"meta"`
	Outputs  []*Output             `json:"outputs"`
}

// CallReceipt for json marshal
type CallReceipt struct {
	GasUsed  uint64                `json:"gasUsed"`
	GasPayer thor.Address          `json:"gasPayer"`
	Paid     *math.HexOrDecimal256 `json:"paid"`
	Reward   *math.HexOrDecimal256 `json:"reward"`
	Reverted bool                  `json:"reverted"`
	TxID     thor.Bytes32          `json:"txID"`
	TxOrigin thor.Address          `json:"txOrigin"`
	Outputs  []*Output             `json:"outputs"`
	VMError  string                `json:"vmError"`
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

// ConvertReceipt convert a raw clause into a json format clause
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

// convertCallReceipt converts a tx.Receipt into a transaction.CallReceipt
func convertCallReceipt(
	txReceipt *tx.Receipt,
	tx *Transaction,
	callAddr *thor.Address,
) (*CallReceipt, error) {
	reward := math.HexOrDecimal256(*txReceipt.Reward)
	paid := math.HexOrDecimal256(*txReceipt.Paid)
	origin := callAddr

	receipt := &CallReceipt{
		GasUsed:  txReceipt.GasUsed,
		GasPayer: txReceipt.GasPayer,
		Paid:     &paid,
		Reward:   &reward,
		Reverted: txReceipt.Reverted,
		TxOrigin: *origin,
		TxID:     tx.ID,
	}
	receipt.Outputs = make([]*Output, len(txReceipt.Outputs))
	for i, output := range txReceipt.Outputs {
		clause := tx.Clauses[i]
		var contractAddr *thor.Address
		if clause.To == nil {
			cAddr := thor.CreateContractAddress(tx.ID, uint32(i), 0)
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

func convertErrorCallReceipt(
	vmErr error,
	tx *Transaction,
	callAddr *thor.Address,
) (*CallReceipt, error) {
	origin := callAddr

	receipt := &CallReceipt{
		Reverted: true,
		TxOrigin: *origin,
		TxID:     tx.ID,
		VMError:  vmErr.Error(),
	}
	receipt.Outputs = make([]*Output, len(tx.Clauses))
	for i := range tx.Clauses {
		clause := tx.Clauses[i]
		var contractAddr *thor.Address
		if clause.To == nil {
			cAddr := thor.CreateContractAddress(tx.ID, uint32(i), 0)
			contractAddr = &cAddr
		}

		receipt.Outputs[i] = &Output{ContractAddress: contractAddr}
	}
	return receipt, nil
}

// ConvertCallTransaction converts a transaction.Transaction into a tx.Transaction
// note: tx.Transaction will not be signed
func ConvertCallTransaction(incomingTx *Transaction, header *block.Header) (*tx.Transaction, error) {
	blockRef, err := tx.NewBlockRefFromHex(incomingTx.BlockRef)
	if err != nil {
		return nil, errors.WithMessage(err, "blockRef")
	}

	convertedTxBuilder := new(tx.Builder).
		ChainTag(incomingTx.ChainTag).
		Features(header.TxsFeatures()).
		Nonce(uint64(incomingTx.Nonce)).
		BlockRef(blockRef).
		Expiration(incomingTx.Expiration).
		GasPriceCoef(incomingTx.GasPriceCoef).
		Gas(incomingTx.Gas).
		DependsOn(incomingTx.DependsOn)

	for _, c := range incomingTx.Clauses {
		value := big.Int(c.Value)
		dataVal, err := hexutil.Decode(c.Data)
		if err != nil {
			return nil, fmt.Errorf("unable to decode clause data: %w", err)
		}
		convertedTxBuilder.Clause(tx.NewClause(c.To).WithValue(&value).WithData(dataVal))
	}

	return convertedTxBuilder.Build(), nil
}

// SendTxResult is the response to the Send Tx method
type SendTxResult struct {
	ID *thor.Bytes32 `json:"id"`
}

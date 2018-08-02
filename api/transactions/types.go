// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package transactions

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
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

func hasKey(m map[string]interface{}, key string) bool {
	for k := range m {
		if strings.ToLower(k) == strings.ToLower(key) {
			return true
		}
	}
	return false
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
	Nonce        math.HexOrDecimal64 `json:"nonce"`
	DependsOn    *thor.Bytes32       `json:"dependsOn"`
	Size         uint32              `json:"size"`
	Meta         TxMeta              `json:"meta"`
}
type UnSignedTx struct {
	ChainTag     uint8               `json:"chainTag"`
	BlockRef     string              `json:"blockRef"`
	Expiration   uint32              `json:"expiration"`
	Clauses      Clauses             `json:"clauses"`
	GasPriceCoef uint8               `json:"gasPriceCoef"`
	Gas          uint64              `json:"gas"`
	DependsOn    *thor.Bytes32       `json:"dependsOn"`
	Nonce        math.HexOrDecimal64 `json:"nonce"`
}

func (ustx *UnSignedTx) decode() (*tx.Transaction, error) {
	txBuilder := new(tx.Builder)
	for _, clause := range ustx.Clauses {
		data, err := hexutil.Decode(clause.Data)
		if err != nil {
			return nil, errors.WithMessage(err, "data")
		}
		v := big.Int(clause.Value)
		txBuilder.Clause(tx.NewClause(clause.To).WithData(data).WithValue(&v))
	}
	blockRef, err := hexutil.Decode(ustx.BlockRef)
	if err != nil {
		return nil, errors.WithMessage(err, "blockRef")
	}
	var bf tx.BlockRef
	copy(bf[:], blockRef[:])

	return txBuilder.ChainTag(ustx.ChainTag).
		BlockRef(bf).
		Expiration(ustx.Expiration).
		Gas(ustx.Gas).
		GasPriceCoef(ustx.GasPriceCoef).
		DependsOn(ustx.DependsOn).
		Nonce(uint64(ustx.Nonce)).
		Build(), nil
}

type SignedTx struct {
	UnSignedTx
	Signature string `json:"signature"`
}

func (stx *SignedTx) decode() (*tx.Transaction, error) {
	tx, err := stx.UnSignedTx.decode()
	if err != nil {
		return nil, err
	}
	sig, err := hexutil.Decode(stx.Signature)
	if err != nil {
		return nil, errors.WithMessage(err, "signature")
	}
	return tx.WithSignature(sig), nil
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
	Meta TxMeta `json:"meta"`
}

//convertTransaction convert a raw transaction into a json format transaction
func convertTransaction(tx *tx.Transaction, header *block.Header, txIndex uint64) (*Transaction, error) {
	//tx signer
	signer, err := tx.Signer()
	if err != nil {
		return nil, err
	}
	cls := make(Clauses, len(tx.Clauses()))
	for i, c := range tx.Clauses() {
		cls[i] = convertClause(c)
	}
	br := tx.BlockRef()
	t := &Transaction{
		ChainTag:     tx.ChainTag(),
		ID:           tx.ID(),
		Origin:       signer,
		BlockRef:     hexutil.Encode(br[:]),
		Expiration:   tx.Expiration(),
		Nonce:        math.HexOrDecimal64(tx.Nonce()),
		Size:         uint32(tx.Size()),
		GasPriceCoef: tx.GasPriceCoef(),
		Gas:          tx.Gas(),
		DependsOn:    tx.DependsOn(),
		Clauses:      cls,
		Meta: TxMeta{
			BlockID:        header.ID(),
			BlockNumber:    header.Number(),
			BlockTimestamp: header.Timestamp(),
		},
	}
	return t, nil
}

type TxMeta struct {
	BlockID        thor.Bytes32 `json:"blockID"`
	BlockNumber    uint32       `json:"blockNumber"`
	BlockTimestamp uint64       `json:"blockTimestamp"`
}

type LogMeta struct {
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
	Meta     LogMeta               `json:"meta"`
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
	signer, err := tx.Signer()
	if err != nil {
		return nil, err
	}
	receipt := &Receipt{
		GasUsed:  txReceipt.GasUsed,
		GasPayer: txReceipt.GasPayer,
		Paid:     &paid,
		Reward:   &reward,
		Reverted: txReceipt.Reverted,
		Meta: LogMeta{
			header.ID(),
			header.Number(),
			header.Timestamp(),
			tx.ID(),
			signer,
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

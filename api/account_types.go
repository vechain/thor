// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"

	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/thor"
)

// Account for marshal account
type Account struct {
	Balance *math.HexOrDecimal256 `json:"balance"`
	Energy  *math.HexOrDecimal256 `json:"energy"`
	HasCode bool                  `json:"hasCode"`
}

// CallData represents contract-call body
type CallData struct {
	Value    *math.HexOrDecimal256 `json:"value"`
	Data     string                `json:"data"`
	Gas      uint64                `json:"gas"`
	GasPrice *math.HexOrDecimal256 `json:"gasPrice"`
	Caller   *thor.Address         `json:"caller"`
}

type GetCodeResult struct {
	Code string `json:"code"`
}

type GetStorageResult struct {
	Value string `json:"value"`
}

type CallResult struct {
	Data      string      `json:"data"`
	Events    []*Event    `json:"events"`
	Transfers []*Transfer `json:"transfers"`
	GasUsed   uint64      `json:"gasUsed"`
	Reverted  bool        `json:"reverted"`
	VMError   string      `json:"vmError"`
}

func ConvertCallResultWithInputGas(vo *runtime.Output, inputGas uint64) *CallResult {
	gasUsed := inputGas - vo.LeftOverGas
	var (
		vmError  string
		reverted bool
	)

	if vo.VMErr != nil {
		reverted = true
		vmError = vo.VMErr.Error()
	}

	events := make([]*Event, len(vo.Events))
	transfers := make([]*Transfer, len(vo.Transfers))

	for j, txEvent := range vo.Events {
		event := &Event{
			Address: txEvent.Address,
			Data:    hexutil.Encode(txEvent.Data),
		}
		event.Topics = make([]thor.Bytes32, len(txEvent.Topics))
		copy(event.Topics, txEvent.Topics)
		events[j] = event
	}
	for j, txTransfer := range vo.Transfers {
		transfer := &Transfer{
			Sender:    txTransfer.Sender,
			Recipient: txTransfer.Recipient,
			Amount:    (*math.HexOrDecimal256)(txTransfer.Amount),
		}
		transfers[j] = transfer
	}

	return &CallResult{
		Data:      hexutil.Encode(vo.Data),
		Events:    events,
		Transfers: transfers,
		GasUsed:   gasUsed,
		Reverted:  reverted,
		VMError:   vmError,
	}
}

type Clause struct {
	To    *thor.Address         `json:"to"`
	Value *math.HexOrDecimal256 `json:"value"`
	Data  string                `json:"data"`
}

// Clauses array of clauses.
type Clauses []*Clause

// BatchCallData executes a batch of codes
type BatchCallData struct {
	Clauses    Clauses               `json:"clauses"`
	Gas        uint64                `json:"gas"`
	GasPrice   *math.HexOrDecimal256 `json:"gasPrice"`
	ProvedWork *math.HexOrDecimal256 `json:"provedWork"`
	Caller     *thor.Address         `json:"caller"`
	GasPayer   *thor.Address         `json:"gasPayer"`
	Expiration uint32                `json:"expiration"`
	BlockRef   string                `json:"blockRef"`
}

type BatchCallResults []*CallResult

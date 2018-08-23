package subscriptions

import (
	"github.com/vechain/thor/thor"
)

type Block struct {
	Number       uint32         `json:"number"`
	ID           thor.Bytes32   `json:"id"`
	Size         uint32         `json:"size"`
	ParentID     thor.Bytes32   `json:"parentID"`
	Timestamp    uint64         `json:"timestamp"`
	GasLimit     uint64         `json:"gasLimit"`
	Beneficiary  thor.Address   `json:"beneficiary"`
	GasUsed      uint64         `json:"gasUsed"`
	TotalScore   uint64         `json:"totalScore"`
	TxsRoot      thor.Bytes32   `json:"txsRoot"`
	StateRoot    thor.Bytes32   `json:"stateRoot"`
	ReceiptsRoot thor.Bytes32   `json:"receiptsRoot"`
	Signer       thor.Address   `json:"signer"`
	IsTrunk      bool           `json:"isTrunk"`
	Transactions []thor.Bytes32 `json:"transactions"`
	Removed      bool           `json:"removed"`
}

// EventFilter contains options for contract event filtering.
type EventFilter struct {
	FromBlock thor.Bytes32 // beginning of the queried range, nil means genesis block
	Addresses thor.Address // restricts matches to events created by specific contracts

	Topic0 thor.Bytes32
	Topic1 thor.Bytes32
	Topic2 thor.Bytes32
	Topic3 thor.Bytes32
	Topic4 thor.Bytes32
}

// TransferFilter contains options for contract transfer filtering.
type TransferFilter struct {
	TxOrigin  *thor.Address // who send transaction
	Sender    *thor.Address // who transferred tokens
	Recipient *thor.Address // who recieved tokens
}

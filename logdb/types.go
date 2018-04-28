package logdb

import (
	"math/big"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//Event represents tx.Event to event to store in db
type Event struct {
	BlockID     thor.Bytes32
	Index       uint32
	BlockNumber uint32
	BlockTime   uint64
	TxID        thor.Bytes32
	TxOrigin    thor.Address //contract caller
	Address     thor.Address // always a contract address
	Topics      [5]*thor.Bytes32
	Data        []byte
}

//newEvent return a format tx event.
func newEvent(header *block.Header, index uint32, txID thor.Bytes32, txOrigin thor.Address, txEvent *tx.Event) *Event {
	ev := &Event{
		BlockID:     header.ID(),
		Index:       index,
		BlockNumber: header.Number(),
		BlockTime:   header.Timestamp(),
		TxID:        txID,
		TxOrigin:    txOrigin,
		Address:     txEvent.Address, // always a contract address
		Data:        txEvent.Data,
	}
	for i := 0; i < len(txEvent.Topics) && i < len(ev.Topics); i++ {
		ev.Topics[i] = &txEvent.Topics[i]
	}
	return ev
}

//Transfer store in db
type Transfer struct {
	BlockID     thor.Bytes32
	Index       uint32
	BlockNumber uint32
	BlockTime   uint64
	TxID        thor.Bytes32
	TxOrigin    thor.Address
	From        thor.Address
	To          thor.Address
	Value       *big.Int
}

//newTransfer return a format transfer
func newTransfer(header *block.Header, index uint32, txID thor.Bytes32, txOrigin thor.Address, transfer *tx.Transfer) *Transfer {
	return &Transfer{
		BlockID:     header.ID(),
		Index:       index,
		BlockNumber: header.Number(),
		BlockTime:   header.Timestamp(),
		TxID:        txID,
		TxOrigin:    txOrigin,
		From:        transfer.Sender,
		To:          transfer.Recipient,
		Value:       transfer.Amount,
	}
}

type RangeType string

const (
	Block RangeType = "Block"
	Time  RangeType = "Time"
)

type OrderType string

const (
	ASC  OrderType = "ASC"
	DESC OrderType = "DESC"
)

type Range struct {
	Unit RangeType `json:"unit"`
	From uint64    `json:"from"`
	To   uint64    `json:"to"`
}

type Options struct {
	Offset uint64 `json:"offset"`
	Limit  uint64 `json:"limit"`
}

//EventFilter filter
type EventFilter struct {
	Address  *thor.Address      `json:"address"` // always a contract address
	TopicSet [][5]*thor.Bytes32 `json:"topicSet"`
	Range    *Range             `json:"range"`
	Options  *Options           `json:"options"`
	Order    OrderType          `json:"order"` //default asc
}

type AddressSet struct {
	TxOrigin *thor.Address `json:"txOrigin"` //who send transaction
	From     *thor.Address `json:"from"`     //who transferred tokens
	To       *thor.Address `json:"to"`       //who recieved tokens
}

type TransferFilter struct {
	TxID        *thor.Bytes32 `json:"txID"`
	AddressSets []*AddressSet `json:"addressSets"`
	Range       *Range        `json:"range"`
	Options     *Options      `json:"options"`
	Order       OrderType     `json:"order"` //default asc
}

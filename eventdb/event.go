package eventdb

import (
	"fmt"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

//Event format tx.Event to store in db
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

//NewEvent return a format tx event.
func NewEvent(header *block.Header, index uint32, txID thor.Bytes32, txOrigin thor.Address, txEvent *tx.Event) *Event {
	l := &Event{
		BlockID:     header.ID(),
		Index:       index,
		BlockNumber: header.Number(),
		BlockTime:   header.Timestamp(),
		TxID:        txID,
		TxOrigin:    txOrigin,
		Address:     txEvent.Address, // always a contract address
		Data:        txEvent.Data,
	}
	for i, topic := range txEvent.Topics {
		// variable topic from range shares the same address, clone topic when need to use topic's pointer
		topic := topic
		l.Topics[i] = &topic
	}
	return l
}

func (e *Event) String() string {
	return fmt.Sprintf(`
		Event(
			blockID:     %v,
			index:	 %v,
			blockNumber: %v,
			blockTime:   %v,
			txID:        %v,
			txOrigin:    %v,
			address:     %v,
			topic0:      %v,
			topic1:      %v,
			topic2:      %v,
			topic3:      %v,
			topic4:      %v,
			data:        0x%x)`,
		e.BlockID,
		e.Index,
		e.BlockNumber,
		e.BlockTime,
		e.TxID,
		e.TxOrigin,
		e.Address,
		e.Topics[0],
		e.Topics[1],
		e.Topics[2],
		e.Topics[3],
		e.Topics[4],
		e.Data)
}

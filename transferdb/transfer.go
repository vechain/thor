package transferdb

import (
	"fmt"
	"math/big"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
)

//Transfer store in db
type Transfer struct {
	BlockID       thor.Bytes32
	TransferIndex uint32
	BlockNumber   uint32
	BlockTime     uint64
	TxID          thor.Bytes32
	TxOrigin      thor.Address
	From          thor.Address
	To            thor.Address
	Value         *big.Int
}

//NewTransfer return a format transfer
func NewTransfer(header *block.Header, transferIndex uint32, txID thor.Bytes32, txOrigin thor.Address, from thor.Address, to thor.Address, Value *big.Int) *Transfer {
	return &Transfer{
		BlockID:       header.ID(),
		TransferIndex: transferIndex,
		BlockNumber:   header.Number(),
		BlockTime:     header.Timestamp(),
		TxID:          txID,
		TxOrigin:      txOrigin,
		From:          from,
		To:            to,
		Value:         Value,
	}
}

func (trans *Transfer) String() string {
	return fmt.Sprintf(`
		Transfer(
			blockID:    	%v,
			transferIndex:	%v,
			blockNumber: 	%v,
			blockTime:  	%v,
			txID:        	%v,
			txOrigin:		%v,
			from:    		%v,
			to:     	 	%v,
			value:      	%v,)`,
		trans.BlockID,
		trans.TransferIndex,
		trans.BlockNumber,
		trans.BlockTime,
		trans.TxID,
		trans.TxOrigin,
		trans.From,
		trans.To,
		trans.Value)
}

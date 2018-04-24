package transferdb

import (
	"fmt"
	"math/big"

	"github.com/vechain/thor/block"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
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
func NewTransfer(header *block.Header, transferIndex uint32, txID thor.Bytes32, txOrigin thor.Address, transferLog *tx.TransferLog) *Transfer {
	return &Transfer{
		BlockID:       header.ID(),
		TransferIndex: transferIndex,
		BlockNumber:   header.Number(),
		BlockTime:     header.Timestamp(),
		TxID:          txID,
		TxOrigin:      txOrigin,
		From:          transferLog.Sender,
		To:            transferLog.Recipient,
		Value:         transferLog.Amount,
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

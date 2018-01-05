package consensus

import (
	"github.com/vechain/thor/block"
)

func validate(parentHeader *block.Header, blk *block.Block) error {
	header := blk.Header()

	if blk.Body().Txs.RootHash() != header.TxsRoot() {
		return errTxsRoot
	}

	if header.GasUsed().Cmp(header.GasLimit()) > 0 {
		return errGasUsed
	}

	if parentHeader.Number()+1 != blk.Number() {
		return errNumber
	}

	if parentHeader.Timestamp() >= blk.Timestamp() {
		return errTimestamp
	}

	return nil
}

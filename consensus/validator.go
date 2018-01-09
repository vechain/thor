package consensus

import (
	"github.com/vechain/thor/block"
)

func validate(preHeader *block.Header, blk *block.Block) error {
	header := blk.Header()

	if blk.Body().Txs.RootHash() != header.TxsRoot() {
		return errTxsRoot
	}

	if header.GasUsed().Cmp(header.GasLimit()) > 0 {
		return errGasUsed
	}

	if preHeader.Number()+1 != blk.Number() {
		return errNumber
	}

	if preHeader.Timestamp() >= blk.Timestamp() {
		return errTimestamp
	}

	return nil
}

package packer

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type TxIterator interface {
	HasNext() bool
	Next() *tx.Transaction

	OnProcessed(txID thor.Hash, err error)
}

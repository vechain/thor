package consensus

import (
	"math/big"

	"github.com/vechain/thor/tx"
)

type processorStub interface {
	processTransaction(*tx.Transaction) (*tx.Receipt, *big.Int, error)
}

type processStub struct {
}

func (p *processStub) processTransaction(*tx.Transaction) (*tx.Receipt, *big.Int, error) {
	return &tx.Receipt{}, new(big.Int), nil
}

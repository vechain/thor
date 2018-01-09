package consensus

import (
	"math/big"

	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

type processorStub interface {
	processTransaction(*tx.Transaction) (*tx.Receipt, *big.Int, error)
	processClause(*tx.Clause) (*vm.Output, error)
}

type processStub struct {
}

func (p *processStub) processTransaction(*tx.Transaction) (*tx.Receipt, *big.Int, error) {
	return &tx.Receipt{}, new(big.Int), nil
}

func (p *processStub) processClause(*tx.Clause) (*vm.Output, error) {
	return &vm.Output{}, nil
}

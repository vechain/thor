package consensus

import (
	"math"
	"math/big"

	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vm"
)

// Consensus check whether the block is verified,
// and predicate which trunk it belong to.
type Consensus struct {
	chain        *chain.Chain
	stateCreator func(thor.Hash) *state.State
	sign         *cry.Signing
}

// New is Consensus factory.
func New(chain *chain.Chain, sign *cry.Signing, stateCreator func(thor.Hash) *state.State) *Consensus {
	return &Consensus{
		chain:        chain,
		sign:         sign,
		stateCreator: stateCreator}
}

// Consent is Consensus's main func.
func (c *Consensus) Consent(blk *block.Block) (isTrunk bool, err error) {
	if blk == nil {
		return false, errors.New("parameter is nil, must be *block.Block")
	}

	preHeader, err := validate(blk, c.chain)
	if err != nil {
		return false, err
	}

	if err = c.verify(blk, preHeader); err != nil {
		return false, err
	}

	return predicateTrunk(blk.Header(), preHeader)
}

func (c *Consensus) verify(blk *block.Block, preHeader *block.Header) error {
	preHash := preHeader.StateRoot()
	state := c.stateCreator(preHash)
	getHash := chain.NewHashGetter(c.chain, preHash).GetHash
	rt := runtime.New(state, preHeader, getHash)
	handler := func(to thor.Address, data []byte) *vm.Output {
		clause := &tx.Clause{
			To:   &to,
			Data: data}
		return rt.Execute(clause, 0, math.MaxUint64, to, &big.Int{}, thor.Hash{})
	}

	header := blk.Header()
	if err := handleProposers(handler, header, c.sign, preHeader); err != nil {
		return err
	}

	energyUsed, err := ProcessBlock(rt, blk, c.sign)
	if err != nil {
		return err
	}

	data := contracts.Energy.PackCharge(header.Beneficiary(), new(big.Int).SetUint64(energyUsed))
	if output := handler(contracts.Energy.Address, data); output.VMErr != nil {
		return errors.Wrap(output.VMErr, "charge energy")
	}

	return checkState(state, header)
}

func predicateTrunk(header *block.Header, preHeader *block.Header) (bool, error) {
	return false, nil
}

package consensus

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/schedule"
	"github.com/vechain/thor/thor"
)

type proposerHandler struct {
	rt        *runtime.Runtime
	header    *block.Header
	preHeader *block.Header
}

func newProposerHandler(
	rt *runtime.Runtime,
	header *block.Header,
	preHeader *block.Header,
) *proposerHandler {
	return &proposerHandler{
		rt:        rt,
		header:    header,
		preHeader: preHeader}
}

func (ph *proposerHandler) handle() error {
	signer, err := ph.header.Signer()
	if err != nil {
		return err
	}

	proposers, err := ph.getProposers()
	if err != nil {
		return err
	}

	updates, err := ph.validateProposers(signer, proposers)
	if err != nil {
		return err
	}

	return ph.updateProposers(updates)
}

func (ph *proposerHandler) getProposers() ([]schedule.Proposer, error) {
	output := handleClause(ph.rt, contracts.Authority.PackProposers())
	if output.VMErr != nil {
		return nil, errors.Wrap(output.VMErr, "get proposers")
	}
	return contracts.Authority.UnpackProposers(output.Value), nil
}

func (ph *proposerHandler) updateProposers(updates []schedule.Proposer) error {
	output := handleClause(ph.rt, contracts.Authority.PackUpdate(updates))
	if output.VMErr != nil {
		return errors.Wrap(output.VMErr, "set absent")
	}
	return nil
}

func (ph *proposerHandler) validateProposers(addr thor.Address, proposers []schedule.Proposer) ([]schedule.Proposer, error) {
	legal, updates, err := schedule.New(
		proposers,
		ph.preHeader.Number(),
		ph.preHeader.Timestamp()).
		Validate(addr, ph.header.Timestamp())

	switch {
	case err != nil:
		return nil, err
	case !legal:
		return nil, errSinger
	case ph.preHeader.TotalScore()+schedule.CalcScore(proposers, updates) != ph.header.TotalScore():
		return nil, errTotalScore
	default:
		return updates, nil
	}
}

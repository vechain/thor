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
	signer    thor.Address
	preHeader *block.Header
}

func newProposerHandler(
	rt *runtime.Runtime,
	header *block.Header,
	signer thor.Address,
	preHeader *block.Header,
) *proposerHandler {
	return &proposerHandler{
		rt:        rt,
		header:    header,
		signer:    signer,
		preHeader: preHeader}
}

func (ph *proposerHandler) handle() error {
	proposers, err := ph.getProposers()
	if err != nil {
		return err
	}

	updates, err := ph.validateProposers(proposers)
	if err != nil {
		return err
	}

	return ph.updateProposers(updates)
}

func (ph *proposerHandler) getProposers() ([]schedule.Proposer, error) {
	output := handleClause(ph.rt, contracts.Authority.Address, contracts.Authority.PackProposers())
	if output.VMErr != nil {
		return nil, errors.Wrap(output.VMErr, "get proposers")
	}
	return contracts.Authority.UnpackProposers(output.Value), nil
}

func (ph *proposerHandler) updateProposers(updates []schedule.Proposer) error {
	output := handleClause(ph.rt, contracts.Authority.Address, contracts.Authority.PackUpdate(updates))
	if output.VMErr != nil {
		return errors.Wrap(output.VMErr, "set absent")
	}
	return nil
}

func (ph *proposerHandler) validateProposers(proposers []schedule.Proposer) ([]schedule.Proposer, error) {
	legal, updates, err := schedule.New(
		proposers,
		ph.preHeader.Number(),
		ph.preHeader.Timestamp()).Validate(ph.signer, ph.header.Timestamp())

	if !legal {
		return nil, errSinger
	}
	if err != nil {
		return nil, err
	}

	if ph.preHeader.TotalScore()+calcScore(proposers, updates) != ph.header.TotalScore() {
		return nil, errTotalScore
	}

	return updates, nil
}

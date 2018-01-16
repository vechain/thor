package consensus

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/schedule"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
)

type proposerHandler struct {
	handler   func(thor.Address, []byte) *vm.Output
	header    *block.Header
	signer    thor.Address
	preHeader *block.Header
}

func newProposerHandler(
	handler func(thor.Address, []byte) *vm.Output,
	header *block.Header,
	signer thor.Address,
	preHeader *block.Header,
) *proposerHandler {
	return &proposerHandler{
		handler:   handler,
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
	output := ph.handler(contracts.Authority.Address, contracts.Authority.PackProposers())
	if output.VMErr != nil {
		return nil, errors.Wrap(output.VMErr, "get proposers")
	}
	return contracts.Authority.UnpackProposers(output.Value), nil
}

func (ph *proposerHandler) updateProposers(updates []schedule.Proposer) error {
	output := ph.handler(contracts.Authority.Address, contracts.Authority.PackUpdate(updates))
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

func calcScore(proposers []schedule.Proposer, updates []schedule.Proposer) uint64 {
	var witness map[thor.Address]bool

	for _, proposer := range proposers {
		if !proposer.IsAbsent() {
			witness[proposer.Address] = true
		}
	}

	for _, update := range updates {
		if update.IsAbsent() {
			delete(witness, update.Address)
		} else {
			witness[update.Address] = true
		}
	}

	return uint64(len(witness))
}

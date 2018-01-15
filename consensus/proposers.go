package consensus

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/cry"
	"github.com/vechain/thor/schedule"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vm"
)

func handleProposers(
	handler func(thor.Address, []byte) *vm.Output,
	header *block.Header,
	sign *cry.Signing,
	preHeader *block.Header) error {

	signer, err := sign.Signer(header)
	if err != nil {
		return err
	}

	proposers, err := getProposers(handler)
	if err != nil {
		return err
	}

	updates, err := validateProposers(proposers, header, signer, preHeader)
	if err != nil {
		return err
	}

	return setProposers(handler, updates)
}

func getProposers(handler func(thor.Address, []byte) *vm.Output) ([]schedule.Proposer, error) {
	output := handler(contracts.Authority.Address, contracts.Authority.PackProposers())
	if output.VMErr != nil {
		return nil, errors.Wrap(output.VMErr, "get proposers")
	}
	return contracts.Authority.UnpackProposers(output.Value), nil
}

func setProposers(handler func(thor.Address, []byte) *vm.Output, updates []schedule.Proposer) error {
	output := handler(contracts.Authority.Address, contracts.Authority.PackUpdate(updates))
	if output.VMErr != nil {
		return errors.Wrap(output.VMErr, "set absent")
	}
	return nil
}

func validateProposers(
	proposers []schedule.Proposer,
	header *block.Header,
	signer thor.Address,
	preHeader *block.Header) ([]schedule.Proposer, error) {

	legal, updates, err := schedule.New(
		proposers,
		preHeader.Number(),
		preHeader.Timestamp()).Validate(signer, header.Timestamp())

	if !legal {
		return nil, errSinger
	}
	if err != nil {
		return nil, err
	}

	if preHeader.TotalScore()+calcScore(proposers, updates) != header.TotalScore() {
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

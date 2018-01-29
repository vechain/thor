package consensus

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	Chain "github.com/vechain/thor/chain"
	"github.com/vechain/thor/contracts"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type proposerHandler struct {
	rt        *runtime.Runtime
	header    *block.Header
	preHeader *block.Header
}

func newProposerHandler(
	chain *Chain.Chain,
	state *state.State,
	header *block.Header,
	preHeader *block.Header,
) *proposerHandler {

	preHash := preHeader.StateRoot()
	rt := runtime.New(state,
		header.Beneficiary(),
		header.Number(),
		header.Timestamp(),
		header.GasLimit(),
		Chain.NewBlockIDGetter(chain, preHash).GetID)

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

func (ph *proposerHandler) getProposers() ([]poa.Proposer, error) {
	output := handleClause(ph.rt, contracts.Authority.PackProposers())
	if output.VMErr != nil {
		return nil, errors.Wrap(output.VMErr, "get proposers")
	}
	return contracts.Authority.UnpackProposers(output.Value), nil
}

func (ph *proposerHandler) updateProposers(updates []poa.Proposer) error {
	output := handleClause(ph.rt, contracts.Authority.PackUpdate(updates))
	if output.VMErr != nil {
		return errors.Wrap(output.VMErr, "set absent")
	}
	return nil
}

func (ph *proposerHandler) validateProposers(addr thor.Address, proposers []poa.Proposer) ([]poa.Proposer, error) {
	targetTime, updates, err := poa.NewScheduler(proposers, ph.preHeader.Number(), ph.preHeader.Timestamp()).
		Schedule(addr, ph.header.Timestamp())

	switch {
	case err != nil:
		return nil, err
	case targetTime != ph.header.Timestamp():
		return nil, errSchedule
	case ph.preHeader.TotalScore()+poa.CalculateScore(proposers, updates) != ph.header.TotalScore():
		return nil, errTotalScore
	default:
		return updates, nil
	}
}

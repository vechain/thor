package consensus

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	Chain "github.com/vechain/thor/chain"
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

	rt := runtime.New(state,
		header.Beneficiary(),
		header.Number(),
		header.Timestamp(),
		header.GasLimit(),
		nil, //safe here
	)

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

	updates, err := ph.validateProposers(signer, builtin.Authority.All(ph.rt.State()))
	if err != nil {
		return err
	}

	for _, proposer := range updates {
		builtin.Authority.Update(ph.rt.State(), proposer.Address, proposer.Status)
	}

	return nil
}

func (ph *proposerHandler) validateProposers(addr thor.Address, proposers []poa.Proposer) ([]poa.Proposer, error) {
	sched, err := poa.NewScheduler(addr, proposers, ph.preHeader.Number(), ph.preHeader.Timestamp())
	if err != nil {
		return nil, err
	}

	targetTime := sched.Schedule(ph.header.Timestamp())
	if !sched.IsTheTime(targetTime) {
		return nil, errSchedule
	}

	updates, score := sched.Updates(targetTime)
	if ph.preHeader.TotalScore()+score != ph.header.TotalScore() {
		return nil, errTotalScore
	}

	return updates, nil
}

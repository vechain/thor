package packer

import (
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/runtime"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// Packer to pack txs and build new blocks.
type Packer struct {
	chain          *chain.Chain
	stateCreator   *state.Creator
	proposer       thor.Address
	beneficiary    thor.Address
	targetGasLimit uint64
}

// New create a new Packer instance.
func New(
	chain *chain.Chain,
	stateCreator *state.Creator,
	proposer thor.Address,
	beneficiary thor.Address) *Packer {

	return &Packer{
		chain,
		stateCreator,
		proposer,
		beneficiary,
		0,
	}
}

// Schedule schedule a packing flow to pack new block upon given parent and clock time.
func (p *Packer) Schedule(parent *block.Header, nowTimestamp uint64) (flow *Flow, err error) {
	state, err := p.stateCreator.NewState(parent.StateRoot())
	if err != nil {
		return nil, errors.Wrap(err, "state")
	}
	endorsement := builtin.Params.Native(state).Get(thor.KeyProposerEndorsement)
	authority := builtin.Authority.Native(state)

	candidates := authority.Candidates()
	proposers := make([]poa.Proposer, 0, len(candidates))
	for _, c := range candidates {
		if state.GetBalance(c.Endorsor).Cmp(endorsement) >= 0 {
			proposers = append(proposers, poa.Proposer{
				Address: c.Signer,
				Active:  c.Active,
			})
		}
	}

	// calc the time when it's turn to produce block
	sched, err := poa.NewScheduler(p.proposer, proposers, parent.Number(), parent.Timestamp())
	if err != nil {
		return nil, err
	}

	newBlockTime := sched.Schedule(nowTimestamp)
	updates, score := sched.Updates(newBlockTime)

	for _, u := range updates {
		authority.Update(u.Address, u.Active)
	}

	traverser := p.chain.NewTraverser(parent.ID())
	runtime := runtime.New(
		state,
		p.beneficiary,
		parent.Number()+1,
		newBlockTime,
		p.gasLimit(parent.GasLimit()),
		func(num uint32) thor.Bytes32 {
			return traverser.Get(num).ID()
		})

	return newFlow(p, parent, runtime, parent.TotalScore()+score, traverser), nil
}

// Mock create a packing flow upon given parent, but with a designated timestamp.
// It will skip the PoA verification and scheduling, and the block produced by
// the returned flow is not in consensus.
func (p *Packer) Mock(parent *block.Header, targetTime uint64) (*Flow, error) {
	state, err := p.stateCreator.NewState(parent.StateRoot())
	if err != nil {
		return nil, errors.Wrap(err, "state")
	}
	traverser := p.chain.NewTraverser(parent.ID())
	runtime := runtime.New(
		state,
		p.beneficiary,
		parent.Number()+1,
		targetTime,
		p.gasLimit(parent.GasLimit()),
		func(num uint32) thor.Bytes32 {
			return traverser.Get(num).ID()
		})

	return newFlow(p, parent, runtime, parent.TotalScore()+1, traverser), nil
}

func (p *Packer) gasLimit(parentGasLimit uint64) uint64 {
	if p.targetGasLimit != 0 {
		return block.GasLimit(p.targetGasLimit).Qualify(parentGasLimit)
	}
	return parentGasLimit
}

// SetTargetGasLimit set target gas limit, the Packer will adjust block gas limit close to
// it as it can.
func (p *Packer) SetTargetGasLimit(gl uint64) {
	p.targetGasLimit = gl
}

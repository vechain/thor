// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"fmt"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/scheduler"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func (c *Consensus) validateAuthorityProposer(header *block.Header, parent *block.Header, st *state.State) (*poaCacher, error) {
	signer, err := header.Signer()
	if err != nil {
		return nil, consensusError(fmt.Sprintf("block signer unavailable: %v", err))
	}

	authority := builtin.Authority.Native(st)
	var candidates *scheduler.Candidates
	if entry, ok := c.validatorsCache.Get(parent.ID()); ok {
		candidates = entry.(*scheduler.Candidates).Copy()
	} else {
		list, err := authority.AllCandidates()
		if err != nil {
			return nil, err
		}
		candidates = scheduler.NewCandidates(list)
	}
	staker := builtin.Staker.Native(st)
	endorsement, err := builtin.Params.Native(st).Get(thor.KeyProposerEndorsement)
	if err != nil {
		return nil, err
	}
	balanceCheck := staker.TransitionPeriodBalanceCheck(c.forkConfig, header.Number(), endorsement)

	proposers, err := candidates.Pick(st, balanceCheck)
	if err != nil {
		return nil, err
	}

	var sched scheduler.Scheduler
	if header.Number() < c.forkConfig.VIP214 {
		sched, err = scheduler.NewPoASchedulerV1(signer, proposers, parent.Number(), parent.Timestamp())
	} else {
		var seed []byte
		seed, err = c.seeder.Generate(header.ParentID())
		if err != nil {
			return nil, err
		}
		sched, err = scheduler.NewPoASchedulerV2(signer, proposers, parent.Number(), parent.Timestamp(), seed)
	}
	if err != nil {
		return nil, consensusError(fmt.Sprintf("block signer invalid: %v %v", signer, err))
	}

	if !sched.IsTheTime(header.Timestamp()) {
		return nil, consensusError(fmt.Sprintf("block timestamp unscheduled: t %v, s %v", header.Timestamp(), signer))
	}

	updates, score := sched.Updates(header.Timestamp())
	if parent.TotalScore()+score != header.TotalScore() {
		return nil, consensusError(fmt.Sprintf("block total score invalid: want %v, have %v", parent.TotalScore()+score, header.TotalScore()))
	}

	for _, u := range updates {
		if _, err := authority.Update(u.Address, u.Active); err != nil {
			return nil, err
		}
		if !candidates.Update(u.Address, u.Active) {
			// should never happen
			panic("something wrong with candidates list")
		}
	}

	return &poaCacher{candidates, c.forkConfig}, nil
}

type poaCacher struct {
	candidates *scheduler.Candidates
	forkConfig *thor.ForkConfig
}

var _ cacher = (*poaCacher)(nil)

func (p *poaCacher) Handle(header *block.Header, receipts tx.Receipts) (any, error) {
	hasAuthorityEvent := func() bool {
		for _, r := range receipts {
			for _, o := range r.Outputs {
				for _, ev := range o.Events {
					if ev.Address == builtin.Authority.Address {
						return true
					}
				}
			}
		}
		return false
	}()

	if hasAuthorityEvent {
		return nil, nil
	}

	// if no endorsor related transfer, or no event emitted from Params contract, the proposers list
	// can be reused
	hasEndorsorEvent := func() bool {
		for _, r := range receipts {
			for _, o := range r.Outputs {
				for _, ev := range o.Events {
					// after HAYABUSA, authorities are allowed to migrate to staker contract,
					// so any staker contract event(AddValidation, StakeIncreased/Decreased/Withdrawn) will need to invalidate cache
					if header.Number() >= p.forkConfig.HAYABUSA && ev.Address == builtin.Staker.Address {
						return true
					}
					if ev.Address == builtin.Params.Address {
						return true
					}
				}
				for _, t := range o.Transfers {
					if p.candidates.IsEndorsor(t.Sender) || p.candidates.IsEndorsor(t.Recipient) {
						return true
					}
				}
			}
		}
		return false
	}()

	if hasEndorsorEvent {
		p.candidates.InvalidateCache()
	}

	return p.candidates, nil
}

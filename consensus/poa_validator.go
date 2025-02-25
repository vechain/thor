// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"fmt"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/poa"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/tx"
)

func (c *Consensus) validateAuthorityProposer(header *block.Header, parent *block.Header, st *state.State) (cacheHandler, error) {
	signer, err := header.Signer()
	if err != nil {
		return nil, consensusError(fmt.Sprintf("block signer unavailable: %v", err))
	}

	authority := builtin.Authority.Native(st)
	var candidates *poa.Candidates
	if entry, ok := c.authorityCache.Get(parent.ID()); ok {
		candidates = entry.(*poa.Candidates).Copy()
	} else {
		list, err := authority.AllCandidates()
		if err != nil {
			return nil, err
		}
		candidates = poa.NewCandidates(list)
	}

	proposers, err := candidates.Pick(st)
	if err != nil {
		return nil, err
	}

	var sched poa.Scheduler
	if header.Number() < c.forkConfig.VIP214 {
		sched, err = poa.NewSchedulerV1(signer, proposers, parent.Number(), parent.Timestamp())
	} else {
		var seed []byte
		seed, err = c.seeder.Generate(header.ParentID())
		if err != nil {
			return nil, err
		}
		sched, err = poa.NewSchedulerV2(signer, proposers, parent.Number(), parent.Timestamp(), seed)
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

	return c.authorityCacheHandler(candidates, header), nil
}

// handleAuthorityEvents checks each block for authority related events, and updates the candidates list accordingly.
func (c *Consensus) authorityCacheHandler(candidates *poa.Candidates, header *block.Header) cacheHandler {
	return func(receipts tx.Receipts) error {
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

		// if no event emitted from Authority contract, it's believed that the candidates list not changed
		if !hasAuthorityEvent {
			// if no endorsor related transfer, or no event emitted from Params contract, the proposers list
			// can be reused
			hasEndorsorEvent := func() bool {
				for _, r := range receipts {
					for _, o := range r.Outputs {
						for _, ev := range o.Events {
							if ev.Address == builtin.Params.Address {
								return true
							}
						}
						for _, t := range o.Transfers {
							if candidates.IsEndorsor(t.Sender) || candidates.IsEndorsor(t.Recipient) {
								return true
							}
						}
					}
				}
				return false
			}()

			if hasEndorsorEvent {
				candidates.InvalidateCache()
			}
			c.authorityCache.Add(header.ID(), candidates)
		}

		return nil
	}
}

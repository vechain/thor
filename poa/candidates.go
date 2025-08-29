// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"slices"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/authority"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// Candidates holds candidates list in memory, and tends to be reused in PoA stage without querying from contract.
type Candidates struct {
	list       []*authority.Candidate
	masters    map[thor.Address]int  // map master address to list index
	endorsors  map[thor.Address]bool // endorsor bitset
	satisfied  []int
	referenced bool
}

// NewCandidates creates candidates list.
func NewCandidates(list []*authority.Candidate) *Candidates {
	masters := make(map[thor.Address]int)
	endorsors := make(map[thor.Address]bool)

	// enable fast check address role
	for i, c := range list {
		masters[c.NodeMaster] = i
		endorsors[c.Endorsor] = true
	}

	return &Candidates{
		list,
		masters,
		endorsors,
		nil,
		false,
	}
}

// Copy make a copy.
func (c *Candidates) Copy() *Candidates {
	c.referenced = true
	cpy := *c
	return &cpy
}

// Pick picks a list of proposers, which satisfy preset conditions.
func (c *Candidates) Pick(state *state.State, checkBalance authority.BalanceChecker) ([]Proposer, error) {
	satisfied := c.satisfied
	if len(satisfied) == 0 {
		mbp, err := builtin.Params.Native(state).Get(thor.KeyMaxBlockProposers)
		if err != nil {
			return nil, err
		}
		maxBlockProposers := mbp.Uint64()
		if maxBlockProposers == 0 || maxBlockProposers > thor.InitialMaxBlockProposers {
			maxBlockProposers = thor.InitialMaxBlockProposers
		}

		satisfied = make([]int, 0, len(c.list))
		for i := 0; i < len(c.list) && uint64(len(satisfied)) < maxBlockProposers; i++ {
			hasBalance, err := checkBalance(c.list[i].NodeMaster, c.list[i].Endorsor)
			if err != nil {
				return nil, err
			}
			if hasBalance {
				satisfied = append(satisfied, i)
			}
		}
		c.satisfied = satisfied
	}

	proposers := make([]Proposer, 0, len(satisfied))
	for _, i := range satisfied {
		proposers = append(proposers, Proposer{
			Address: c.list[i].NodeMaster,
			Active:  c.list[i].Active,
		})
	}
	return proposers, nil
}

// Update update candidate activity status, by its master address.
// It returns false if the given address is not a master.
func (c *Candidates) Update(addr thor.Address, active bool) bool {
	if i, exist := c.masters[addr]; exist {
		// something like COW
		if c.referenced {
			// shallow copy the list
			c.list = slices.Clone(c.list)
			c.referenced = false
		}
		cpy := *c.list[i]
		cpy.Active = active
		c.list[i] = &cpy
		return true
	}
	return false
}

// IsEndorsor returns whether an address is an endorsor.
func (c *Candidates) IsEndorsor(addr thor.Address) bool {
	return c.endorsors[addr]
}

// InvalidateCache invalidate the result cache of Pick method.
func (c *Candidates) InvalidateCache() {
	c.satisfied = nil
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package authority

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var (
	headKey = thor.Blake2b([]byte("head"))
	tailKey = thor.Blake2b([]byte("tail"))
)

// Authority implements native methods of `Authority` contract.
type Authority struct {
	addr  thor.Address
	state *state.State
}

// New create a new instance.
func New(addr thor.Address, state *state.State) *Authority {
	return &Authority{addr, state}
}

func (a *Authority) getEntry(nodeMaster thor.Address) *entry {
	var entry entry
	a.state.DecodeStorage(a.addr, thor.BytesToBytes32(nodeMaster[:]), func(raw []byte) error {
		if len(raw) == 0 {
			return nil
		}
		return rlp.DecodeBytes(raw, &entry)
	})
	return &entry
}

func (a *Authority) setEntry(nodeMaster thor.Address, entry *entry) {
	a.state.EncodeStorage(a.addr, thor.BytesToBytes32(nodeMaster[:]), func() ([]byte, error) {
		if entry.IsEmpty() {
			return nil, nil
		}
		return rlp.EncodeToBytes(entry)
	})
}

func (a *Authority) getAddressPtr(key thor.Bytes32) (addr *thor.Address) {
	a.state.DecodeStorage(a.addr, key, func(raw []byte) error {
		if len(raw) == 0 {
			return nil
		}
		return rlp.DecodeBytes(raw, &addr)
	})
	return
}

func (a *Authority) setAddressPtr(key thor.Bytes32, addr *thor.Address) {
	a.state.EncodeStorage(a.addr, key, func() ([]byte, error) {
		if addr == nil {
			return nil, nil
		}
		return rlp.EncodeToBytes(addr)
	})
}

// Get get candidate by node master address.
func (a *Authority) Get(nodeMaster thor.Address) (listed bool, endorsor thor.Address, identity thor.Bytes32, active bool) {
	entry := a.getEntry(nodeMaster)
	if entry.IsLinked() {
		return true, entry.Endorsor, entry.Identity, entry.Active
	}
	// if it's the only node, IsLinked will be false.
	// check whether it's the head.
	ptr := a.getAddressPtr(headKey)
	listed = ptr != nil && *ptr == nodeMaster
	return listed, entry.Endorsor, entry.Identity, entry.Active
}

// Add add a new candidate.
func (a *Authority) Add(nodeMaster thor.Address, endorsor thor.Address, identity thor.Bytes32) bool {
	entry := a.getEntry(nodeMaster)
	if !entry.IsEmpty() {
		return false
	}

	entry.Endorsor = endorsor
	entry.Identity = identity
	entry.Active = true // defaults to active

	tailPtr := a.getAddressPtr(tailKey)
	entry.Prev = tailPtr

	a.setAddressPtr(tailKey, &nodeMaster)
	if tailPtr == nil {
		a.setAddressPtr(headKey, &nodeMaster)
	} else {
		tailEntry := a.getEntry(*tailPtr)
		tailEntry.Next = &nodeMaster
		a.setEntry(*tailPtr, tailEntry)
	}

	a.setEntry(nodeMaster, entry)
	return true
}

// Revoke revoke candidate by given node master address.
// The entry is not removed, but set unlisted and inactive.
func (a *Authority) Revoke(nodeMaster thor.Address) bool {
	entry := a.getEntry(nodeMaster)
	if !entry.IsLinked() {
		return false
	}

	if entry.Prev == nil {
		a.setAddressPtr(headKey, entry.Next)
	} else {
		prevEntry := a.getEntry(*entry.Prev)
		prevEntry.Next = entry.Next
		a.setEntry(*entry.Prev, prevEntry)
	}

	if entry.Next == nil {
		a.setAddressPtr(tailKey, entry.Prev)
	} else {
		nextEntry := a.getEntry(*entry.Next)
		nextEntry.Prev = entry.Prev
		a.setEntry(*entry.Next, nextEntry)
	}

	entry.Next = nil
	entry.Prev = nil     // unlist
	entry.Active = false // and set to inactive
	a.setEntry(nodeMaster, entry)
	return true
}

// Update update candidate's status.
func (a *Authority) Update(nodeMaster thor.Address, active bool) bool {
	entry := a.getEntry(nodeMaster)
	if !entry.IsLinked() {
		return false
	}
	entry.Active = active
	a.setEntry(nodeMaster, entry)
	return true
}

// Candidates picks a batch of candidates up to limit, that satisfy given endorsement.
func (a *Authority) Candidates(endorsement *big.Int, limit uint64) []*Candidate {
	ptr := a.getAddressPtr(headKey)
	candidates := make([]*Candidate, 0, limit)
	for ptr != nil && uint64(len(candidates)) < limit {
		entry := a.getEntry(*ptr)
		if bal := a.state.GetBalance(entry.Endorsor); bal.Cmp(endorsement) >= 0 {
			candidates = append(candidates, &Candidate{
				NodeMaster: *ptr,
				Endorsor:   entry.Endorsor,
				Identity:   entry.Identity,
				Active:     entry.Active,
			})
		}
		ptr = entry.Next
	}
	return candidates
}

// First returns node master address of first entry.
func (a *Authority) First() *thor.Address {
	return a.getAddressPtr(headKey)
}

// Next returns address of next node master address after given node master address.
func (a *Authority) Next(nodeMaster thor.Address) *thor.Address {
	return a.getEntry(nodeMaster).Next
}

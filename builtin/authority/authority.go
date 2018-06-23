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

func (a *Authority) getEntry(addr thor.Address) *entry {
	var entry entry
	a.state.DecodeStorage(a.addr, thor.BytesToBytes32(addr[:]), func(raw []byte) error {
		if len(raw) == 0 {
			return nil
		}
		return rlp.DecodeBytes(raw, &entry)
	})
	return &entry
}

func (a *Authority) setEntry(addr thor.Address, entry *entry) {
	a.state.EncodeStorage(a.addr, thor.BytesToBytes32(addr[:]), func() ([]byte, error) {
		if entry.IsEmpty() {
			return nil, nil
		}
		return rlp.EncodeToBytes(entry)
	})
}

func (a *Authority) getAddressPtr(key thor.Bytes32) (ptr *thor.Address) {
	a.state.DecodeStorage(a.addr, key, func(raw []byte) error {
		if len(raw) == 0 {
			return nil
		}
		return rlp.DecodeBytes(raw, &ptr)
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

// Get get candidate by signer address.
func (a *Authority) Get(signer thor.Address) (*Candidate, bool) {
	entry := a.getEntry(signer)
	return &Candidate{
		Signer:   signer,
		Endorsor: entry.Endorsor,
		Identity: entry.Identity,
		Active:   entry.Active,
	}, true
}

func (a *Authority) getAndSet(signer thor.Address, f func(entry *entry) bool) bool {
	entry := a.getEntry(signer)
	if !f(entry) {
		return false
	}
	a.setEntry(signer, entry)
	return true
}

// Add add a new candidate.
func (a *Authority) Add(candidate *Candidate) bool {
	tailPtr := a.getAddressPtr(tailKey)

	if !a.getAndSet(candidate.Signer, func(entry *entry) bool {
		if !entry.IsEmpty() {
			return false
		}
		entry.Endorsor = candidate.Endorsor
		entry.Identity = candidate.Identity
		entry.Active = candidate.Active
		entry.Prev = tailPtr
		return true
	}) {
		return false
	}
	a.setAddressPtr(tailKey, &candidate.Signer)

	if tailPtr == nil {
		a.setAddressPtr(headKey, &candidate.Signer)
	} else {
		a.getAndSet(*tailPtr, func(entry *entry) bool {
			entry.Next = &candidate.Signer
			return true
		})
	}
	return true
}

// Remove remove an candidate by given signer address.
func (a *Authority) Remove(signer thor.Address) bool {
	return a.getAndSet(signer, func(ent *entry) bool {
		if ent.IsEmpty() {
			return false
		}
		if ent.Prev == nil {
			a.setAddressPtr(headKey, ent.Next)
		} else {
			a.getAndSet(*ent.Prev, func(prev *entry) bool {
				prev.Next = ent.Next
				return true
			})
		}

		if ent.Next == nil {
			a.setAddressPtr(tailKey, ent.Prev)
		} else {
			a.getAndSet(*ent.Next, func(next *entry) bool {
				next.Prev = ent.Prev
				return true
			})
		}

		*ent = entry{}
		return true
	})
}

// Update update candidate's status.
func (a *Authority) Update(signer thor.Address, active bool) bool {
	return a.getAndSet(signer, func(entry *entry) bool {
		if entry.IsEmpty() {
			return false
		}
		entry.Active = active
		return true
	})
}

// Candidates picks a batch of candidates up to limit, that satisfy given endorsement.
func (a *Authority) Candidates(endorsement *big.Int, limit uint64) []*Candidate {
	ptr := a.getAddressPtr(headKey)
	candidates := make([]*Candidate, 0, limit)
	for ptr != nil && uint64(len(candidates)) < limit {
		entry := a.getEntry(*ptr)
		if bal := a.state.GetBalance(entry.Endorsor); bal.Cmp(endorsement) >= 0 {
			candidates = append(candidates, &Candidate{
				Signer:   *ptr,
				Endorsor: entry.Endorsor,
				Identity: entry.Identity,
				Active:   entry.Active,
			})
		}
		ptr = entry.Next
	}
	return candidates
}

// First returns signer address of first entry.
func (a *Authority) First() *thor.Address {
	return a.getAddressPtr(headKey)
}

// Next returns address of next signer after given signer.
func (a *Authority) Next(signer thor.Address) *thor.Address {
	return a.getEntry(signer).Next
}

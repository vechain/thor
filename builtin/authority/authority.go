// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package authority

import (
	"math/big"

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

func (a *Authority) getStorage(key thor.Bytes32, val interface{}) {
	a.state.GetStructuredStorage(a.addr, key, val)
}

func (a *Authority) setStorage(key thor.Bytes32, val interface{}) {
	a.state.SetStructuredStorage(a.addr, key, val)
}

// Get get candidate by signer address.
func (a *Authority) Get(signer thor.Address) (*Candidate, bool) {
	var entry entry
	a.getStorage(thor.BytesToBytes32(signer[:]), &entry)
	if entry.IsEmpty() {
		return nil, false
	}
	return &Candidate{
		Signer:   signer,
		Endorsor: entry.Endorsor,
		Identity: entry.Identity,
		Active:   entry.Active,
	}, true
}

func (a *Authority) getAndSet(signer thor.Address, f func(entry *entry) bool) bool {
	key := thor.BytesToBytes32(signer[:])
	var entry entry
	a.getStorage(key, &entry)
	if !f(&entry) {
		return false
	}
	a.setStorage(key, &entry)
	return true
}

// Add add a new candidate.
func (a *Authority) Add(candidate *Candidate) bool {
	var tail addressPtr
	a.getStorage(tailKey, &tail)

	if !a.getAndSet(candidate.Signer, func(entry *entry) bool {
		if !entry.IsEmpty() {
			return false
		}
		entry.Endorsor = candidate.Endorsor
		entry.Identity = candidate.Identity
		entry.Active = candidate.Active
		entry.Prev = tail.Address
		return true
	}) {
		return false
	}

	a.setStorage(tailKey, &addressPtr{&candidate.Signer})
	if tail.Address == nil {
		a.setStorage(headKey, &addressPtr{&candidate.Signer})
	} else {
		a.getAndSet(*tail.Address, func(entry *entry) bool {
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
			a.setStorage(headKey, &addressPtr{ent.Next})
		} else {
			a.getAndSet(*ent.Prev, func(prev *entry) bool {
				prev.Next = ent.Next
				return true
			})
		}

		if ent.Next == nil {
			a.setStorage(tailKey, &addressPtr{ent.Prev})
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
	var ptr addressPtr
	a.getStorage(headKey, &ptr)
	candidates := make([]*Candidate, 0, limit)
	for ptr.Address != nil && uint64(len(candidates)) < limit {
		var entry entry
		a.getStorage(thor.BytesToBytes32(ptr.Address[:]), &entry)
		if bal := a.state.GetBalance(entry.Endorsor); bal.Cmp(endorsement) >= 0 {
			candidates = append(candidates, &Candidate{
				Signer:   *ptr.Address,
				Endorsor: entry.Endorsor,
				Identity: entry.Identity,
				Active:   entry.Active,
			})
		}
		ptr.Address = entry.Next
	}
	return candidates
}

// First returns signer address of first entry.
func (a *Authority) First() *thor.Address {
	var ptr addressPtr
	a.getStorage(headKey, &ptr)
	return ptr.Address
}

// Next returns address of next signer after given signer.
func (a *Authority) Next(signer thor.Address) *thor.Address {
	var entry entry
	a.getStorage(thor.BytesToBytes32(signer[:]), &entry)
	return entry.Next
}

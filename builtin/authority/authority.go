package authority

import (
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var (
	headKey = thor.Bytes32(crypto.Keccak256Hash([]byte("head")))
	tailKey = thor.Bytes32(crypto.Keccak256Hash([]byte("tail")))
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
	a.state.GetStructedStorage(a.addr, key, val)
}

func (a *Authority) setStorage(key thor.Bytes32, val interface{}) {
	a.state.SetStructedStorage(a.addr, key, val)
}

// Get get entry by signer address.
func (a *Authority) Get(signer thor.Address) *Entry {
	var entry Entry
	a.getStorage(thor.BytesToBytes32(signer.Bytes()), &entry)
	return &entry
}

func (a *Authority) getAndSet(signer thor.Address, cb func(*Entry) bool) bool {
	key := thor.BytesToBytes32(signer.Bytes())
	var entry Entry
	a.getStorage(key, &entry)
	if !cb(&entry) {
		return false
	}
	a.setStorage(key, &entry)
	return true
}

// Add add a new entry.
// Returns false if already exists.
func (a *Authority) Add(signer thor.Address, endorsor thor.Address, identity thor.Bytes32) bool {
	var tail addressPtr
	a.getStorage(tailKey, &tail)

	if !a.getAndSet(signer, func(entry *Entry) bool {
		if !entry.IsEmpty() {
			return false
		}
		*entry = Entry{
			Endorsor: endorsor,
			Identity: identity,
			Prev:     tail.Address,
		}
		return true
	}) {
		return false
	}

	if tail.Address == nil {
		a.setStorage(headKey, &addressPtr{&signer})
	} else {
		a.getAndSet(*tail.Address, func(entry *Entry) bool {
			entry.Next = &signer
			return true
		})
	}
	a.setStorage(tailKey, &addressPtr{&signer})
	return true
}

// Remove remove an entry by given signer address.
func (a *Authority) Remove(signer thor.Address) bool {
	return a.getAndSet(signer, func(entry *Entry) bool {
		if entry.IsEmpty() {
			return false
		}
		if entry.Prev == nil {
			a.setStorage(headKey, &addressPtr{entry.Next})
		} else {
			a.getAndSet(*entry.Prev, func(prev *Entry) bool {
				prev.Next = entry.Next
				return true
			})
		}

		if entry.Next == nil {
			a.setStorage(tailKey, &addressPtr{entry.Prev})
		} else {
			a.getAndSet(*entry.Next, func(next *Entry) bool {
				next.Prev = entry.Prev
				return true
			})
		}

		*entry = Entry{}
		return true
	})
}

// Update update entry status.
func (a *Authority) Update(signer thor.Address, active bool) bool {
	return a.getAndSet(signer, func(entry *Entry) bool {
		if entry.IsEmpty() {
			return false
		}
		entry.Active = active
		return true
	})
}

// Candidates picks candidates up to thor.MaxBlockProposers.
func (a *Authority) Candidates() []*Candidate {
	var ptr addressPtr
	a.getStorage(headKey, &ptr)
	candidates := make([]*Candidate, 0, thor.MaxBlockProposers)
	for ptr.Address != nil && uint64(len(candidates)) < thor.MaxBlockProposers {
		p := a.Get(*ptr.Address)
		candidates = append(candidates, &Candidate{*ptr.Address, p.Endorsor, p.Identity, p.Active})
		ptr.Address = p.Next
	}
	return candidates
}

// First returns signer address of first entry.
func (a *Authority) First() thor.Address {
	var ptr addressPtr
	a.getStorage(headKey, &ptr)
	if ptr.Address == nil {
		return thor.Address{}
	}
	return *ptr.Address
}

package builtin

import (
	"github.com/vechain/thor/builtin/sslot"
	"github.com/vechain/thor/poa"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// Authority binder of `Authority` contract.
var Authority = func() *authority {
	c := loadContract("Authority")
	return &authority{
		c,
		sslot.NewArray(c.Address, 100),
		sslot.NewMap(c.Address, 101),
		sslot.NewMap(c.Address, 102),
	}
}()

type authority struct {
	*contract
	array    *sslot.Array // slot to store whitelist
	indexMap *sslot.Map   // slot to map address to index of proposer in whitelist
	idMap    *sslot.Map   // slot to map address to identity
}

func (a *authority) indexOf(state *state.State, addr thor.Address) (index uint64) {
	a.indexMap.ForKey(addr).Load(state, &index)
	return
}

// Add add a new proposer.
// Returns false if already exists or proposer count exceeds limit.
func (a *authority) Add(state *state.State, addr thor.Address, identity thor.Hash) bool {
	if a.Count(state) >= thor.MaxBlockProposers {
		return false
	}
	if a.indexOf(state, addr) > 0 {
		// aready exists
		return false
	}
	length := a.array.Append(state, &stgProposer{Address: addr})
	a.indexMap.ForKey(addr).Save(state, length)

	a.idMap.ForKey(addr).Save(state, identity)
	return true
}

// Remove remove a proposer.
// returns false if not found.
func (a *authority) Remove(state *state.State, addr thor.Address) bool {
	index := a.indexOf(state, addr)
	if index == 0 {
		// not found
		return false
	}
	a.indexMap.ForKey(addr).Save(state, nil)
	length := a.array.Len(state)
	if length != index {
		var last stgProposer
		// move last elem to gap of removed one
		a.array.ForIndex(length-1).Load(state, &last)
		a.array.ForIndex(index-1).Save(state, &last)
	}

	// will clear last elem
	a.array.SetLen(state, length-1)
	a.idMap.ForKey(addr).Save(state, nil)
	return true

}

// Status get status of a proposer.
func (a *authority) Status(state *state.State, addr thor.Address) (listed bool, identity thor.Hash, status uint32) {
	index := a.indexOf(state, addr)
	if index == 0 {
		// not found
		return false, thor.Hash{}, 0
	}

	var p stgProposer
	a.array.ForIndex(index-1).Load(state, &p)

	a.idMap.ForKey(addr).Load(state, &identity)
	return true, identity, p.Status
}

// Update update proposer status.
func (a *authority) Update(state *state.State, addr thor.Address, status uint32) bool {
	index := a.indexOf(state, addr)
	if index == 0 {
		// not found
		return false
	}
	a.array.ForIndex(index-1).Save(state, &stgProposer{addr, status})
	return true
}

// Count returns count of proposers added.
func (a *authority) Count(state *state.State) uint64 {
	return a.array.Len(state)
}

// All returns all proposers.
func (a *authority) All(state *state.State) []poa.Proposer {
	count := a.Count(state)
	all := make([]poa.Proposer, 0, count)
	for i := uint64(0); i < count; i++ {
		var p stgProposer
		a.array.ForIndex(i).Load(state, &p)
		all = append(all, poa.Proposer(p))
	}
	return all
}

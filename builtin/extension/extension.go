package extension

import (
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// Extension implements native methods of `Extension` contract.
type Extension struct {
	addr  thor.Address
	state *state.State
}

// New create a new instance.
func New(addr thor.Address, state *state.State) *Extension {
	return &Extension{addr, state}
}

// Blake2b256 implemented as a native contract.
func (e *Extension) Blake2b256(data ...[]byte) (b32 thor.Bytes32) {
	hash := thor.NewBlake2b()
	for _, b := range data {
		hash.Write(b)
	}
	hash.Sum(b32[:0])
	return
}

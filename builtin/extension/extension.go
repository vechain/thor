// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package extension

import (
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

// Extension implements native methods of `Extension` contract.
type Extension struct {
	addr   thor.Address
	state  *state.State
	seeker *chain.Seeker
}

// New create a new instance.
func New(addr thor.Address, state *state.State, seeker *chain.Seeker) *Extension {
	return &Extension{addr, state, seeker}
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

// GetBlockIDByNum implements getting block id by give num.
func (e *Extension) GetBlockIDByNum(num uint32) thor.Bytes32 {
	return e.seeker.GetID(num)
}

// GetBlockHeaderByNum implements getting block header by given num.
func (e *Extension) GetBlockHeaderByNum(num uint32) *block.Header {
	id := e.GetBlockIDByNum(num)
	return e.seeker.GetHeader(id)
}

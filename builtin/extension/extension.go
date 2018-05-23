// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package extension

import (
	"encoding/binary"

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

// SetBlockNumAndID implements storing block num and id into contract storage.
func (e *Extension) SetBlockNumAndID(id thor.Bytes32) {
	key := thor.BytesToBytes32(id[:4])
	e.state.SetStorage(e.addr, key, id)
}

// GetBlockIDByNum implements getting block id by num.
func (e *Extension) GetBlockIDByNum(num uint32) (b32 thor.Bytes32) {
	var key thor.Bytes32
	binary.BigEndian.PutUint32(key[28:], num)
	b32 = e.state.GetStorage(e.addr, key)
	return
}

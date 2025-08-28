// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package params

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

// Params binder of `Params` contract.
type Params struct {
	addr  thor.Address
	state *state.State
}

func New(addr thor.Address, state *state.State) *Params {
	return &Params{addr, state}
}

// Get native way to get param.
func (p *Params) Get(key thor.Bytes32) (value *big.Int, err error) {
	err = p.state.DecodeStorage(p.addr, key, func(raw []byte) error {
		if len(raw) == 0 {
			value = &big.Int{}
			return nil
		}
		return rlp.DecodeBytes(raw, &value)
	})
	return
}

// Set native way to set param.
func (p *Params) Set(key thor.Bytes32, value *big.Int) error {
	if key == thor.KeyMaxBlockProposers && thor.IsLocked() {
		return fmt.Errorf("cannot set max-block-proposers config param")
	}

	return p.state.EncodeStorage(p.addr, key, func() ([]byte, error) {
		if value.Sign() == 0 {
			return nil, nil
		}
		return rlp.EncodeToBytes(value)
	})
}

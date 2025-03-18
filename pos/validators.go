// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pos

import (
	"maps"

	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

type Validators struct {
	mapping    map[thor.Address]*staker.Validator
	referenced bool
}

func NewValidators(mapping map[thor.Address]*staker.Validator) *Validators {
	return &Validators{mapping: mapping}
}

func (v *Validators) Pick(state *state.State) (map[thor.Address]*staker.Validator, error) {
	leaders := v.mapping
	if len(leaders) == 0 {
		var err error
		leaders, err = builtin.Staker.Native(state).LeaderGroup()
		if err != nil {
			return nil, err
		}
		v.mapping = leaders
		v.referenced = false
	}
	return leaders, nil
}

func (v *Validators) Copy() *Validators {
	v.referenced = true
	cpy := *v
	return &cpy
}

func (v *Validators) beforeUpdate() {
	if v.referenced {
		copied := make(map[thor.Address]*staker.Validator, len(v.mapping))
		maps.Copy(copied, v.mapping)
		v.mapping = copied
		v.referenced = false
	}
}

func (v *Validators) Remove(addr thor.Address) {
	v.beforeUpdate()
	delete(v.mapping, addr)
}

func (v *Validators) Add(addr thor.Address, validator *staker.Validator) {
	v.beforeUpdate()
	v.mapping[addr] = validator
}

// InvalidateCache invalidates the cache.
func (v *Validators) InvalidateCache() {
	v.mapping = nil
}

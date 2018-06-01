// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package energy

import (
	"math/big"

	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var (
	initialSupplyKey = thor.Blake2b([]byte("initial-supply"))
	totalAddSubKey   = thor.Blake2b([]byte("total-add-sub"))
)

// Energy implements energy operations.
type Energy struct {
	addr      thor.Address
	state     *state.State
	blockTime uint64
}

// New creates a new energy instance.
func New(addr thor.Address, state *state.State, blockTime uint64) *Energy {
	return &Energy{addr, state, blockTime}
}

func (e *Energy) getStorage(key thor.Bytes32, val interface{}) {
	e.state.GetStructuredStorage(e.addr, key, val)
}

func (e *Energy) setStorage(key thor.Bytes32, val interface{}) {
	e.state.SetStructuredStorage(e.addr, key, val)
}

// SetInitialSupply set initial token and energy supply, to help calculating total energy supply.
func (e *Energy) SetInitialSupply(token *big.Int, energy *big.Int) {
	e.setStorage(initialSupplyKey, &initialSupply{
		Token:     token,
		Energy:    energy,
		BlockTime: e.blockTime,
	})
}

// TokenTotalSupply returns total supply of VET.
func (e *Energy) TokenTotalSupply() *big.Int {
	// that's totalGrown + totalAdd - totalSub
	var init initialSupply
	e.getStorage(initialSupplyKey, &init)

	return init.Token
}

// TotalSupply returns total supply of energy.
func (e *Energy) TotalSupply() *big.Int {
	// that's totalGrown + totalAdd - totalSub
	var init initialSupply
	e.getStorage(initialSupplyKey, &init)

	// calc grown energy for total token supply
	acc := state.Account{Balance: init.Token, Energy: init.Energy, BlockTime: init.BlockTime}
	return acc.CalcEnergy(e.blockTime)
}

// TotalBurned returns energy totally burned.
func (e *Energy) TotalBurned() *big.Int {
	var total totalAddSub
	e.getStorage(totalAddSubKey, &total)
	return new(big.Int).Sub(total.TotalSub, total.TotalAdd)
}

// Get returns energy of an account at given block time.
func (e *Energy) Get(addr thor.Address) *big.Int {
	return e.state.GetEnergy(addr, e.blockTime)
}

// Add add amount of energy to given address.
func (e *Energy) Add(addr thor.Address, amount *big.Int) {
	eng := e.state.GetEnergy(addr, e.blockTime)
	if amount.Sign() != 0 {
		var total totalAddSub
		e.getStorage(totalAddSubKey, &total)
		e.setStorage(totalAddSubKey, &totalAddSub{
			TotalAdd: new(big.Int).Add(total.TotalAdd, amount),
			TotalSub: total.TotalSub,
		})

		e.state.SetEnergy(addr, new(big.Int).Add(eng, amount), e.blockTime)
	} else {
		e.state.SetEnergy(addr, eng, e.blockTime)
	}
}

// Sub sub amount of energy from given address.
// False is returned if no enough energy.
func (e *Energy) Sub(addr thor.Address, amount *big.Int) bool {
	eng := e.state.GetEnergy(addr, e.blockTime)
	if amount.Sign() != 0 {
		if eng.Cmp(amount) < 0 {
			return false
		}

		var total totalAddSub
		e.getStorage(totalAddSubKey, &total)
		e.setStorage(totalAddSubKey, &totalAddSub{
			TotalAdd: total.TotalAdd,
			TotalSub: new(big.Int).Add(total.TotalSub, amount),
		})

		e.state.SetEnergy(addr, new(big.Int).Sub(eng, amount), e.blockTime)
	} else {
		e.state.SetEnergy(addr, eng, e.blockTime)
	}
	return true
}

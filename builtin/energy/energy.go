// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package energy

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var (
	initialSupplyKey = thor.Blake2b([]byte("initial-supply"))
	totalAddSubKey   = thor.Blake2b([]byte("total-add-sub"))
)

type initialSupply struct {
	Token     *big.Int
	Energy    *big.Int
	BlockTime uint64
}

var _ state.StorageDecoder = (*initialSupply)(nil)
var _ state.StorageEncoder = (*initialSupply)(nil)

// Encode implements state.StorageEncoder.
func (i *initialSupply) Encode() ([]byte, error) {
	if i.Token.Sign() == 0 && i.Energy.Sign() == 0 && i.BlockTime == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(i)
}

// Decode implements state.StorageDecoder.
func (i *initialSupply) Decode(data []byte) error {
	if len(data) == 0 {
		*i = initialSupply{
			&big.Int{},
			&big.Int{},
			0,
		}
		return nil
	}
	return rlp.DecodeBytes(data, i)
}

type totalAddSub struct {
	TotalAdd *big.Int
	TotalSub *big.Int
}

var _ state.StorageDecoder = (*totalAddSub)(nil)
var _ state.StorageEncoder = (*totalAddSub)(nil)

// Encode implements state.StorageEncoder.
func (t *totalAddSub) Encode() ([]byte, error) {
	if t.TotalAdd.Sign() == 0 && t.TotalSub.Sign() == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(t)
}

// Decode implements state.StorageDecoder.
func (t *totalAddSub) Decode(data []byte) error {
	if len(data) == 0 {
		*t = totalAddSub{
			&big.Int{},
			&big.Int{},
		}
		return nil
	}
	return rlp.DecodeBytes(data, t)
}

// Energy implements energy operations.
type Energy struct {
	addr  thor.Address
	state *state.State
}

// New creates a new energy instance.
func New(addr thor.Address, state *state.State) *Energy {
	return &Energy{addr, state}
}

func (e *Energy) getStorage(key thor.Bytes32, val interface{}) {
	e.state.GetStructuredStorage(e.addr, key, val)
}

func (e *Energy) setStorage(key thor.Bytes32, val interface{}) {
	e.state.SetStructuredStorage(e.addr, key, val)
}

// SetInitialSupply set initial token and energy supply, to help calculating total energy supply.
func (e *Energy) SetInitialSupply(token *big.Int, energy *big.Int, blockTime uint64) {
	e.setStorage(initialSupplyKey, &initialSupply{
		Token:     token,
		Energy:    energy,
		BlockTime: blockTime,
	})
}

// GetTotalSupply returns total supply of energy.
func (e *Energy) GetTotalSupply(blockTime uint64) *big.Int {
	// that's totalGrown + totalAdd - totalSub
	var init initialSupply
	e.getStorage(initialSupplyKey, &init)

	// calc grown energy for total token supply
	energyState := state.EnergyState{Energy: init.Energy, BlockTime: init.BlockTime}
	return energyState.CalcEnergy(init.Token, blockTime)
}

// GetTotalBurned returns energy totally burned.
func (e *Energy) GetTotalBurned() *big.Int {
	var total totalAddSub
	e.getStorage(totalAddSubKey, &total)
	return new(big.Int).Sub(total.TotalSub, total.TotalAdd)
}

// GetBalance returns energy balance of an account at given block time.
func (e *Energy) GetBalance(addr thor.Address, blockTime uint64) *big.Int {
	return e.state.GetEnergy(addr, blockTime)
}

func (e *Energy) AddBalance(addr thor.Address, amount *big.Int, blockTime uint64) {
	bal := e.state.GetEnergy(addr, blockTime)
	if amount.Sign() != 0 {
		var total totalAddSub
		e.getStorage(totalAddSubKey, &total)
		e.setStorage(totalAddSubKey, &totalAddSub{
			TotalAdd: new(big.Int).Add(total.TotalAdd, amount),
			TotalSub: total.TotalSub,
		})

		e.state.SetEnergy(addr, new(big.Int).Add(bal, amount), blockTime)
	} else {
		e.state.SetEnergy(addr, bal, blockTime)
	}
}

func (e *Energy) SubBalance(addr thor.Address, amount *big.Int, blockTime uint64) bool {
	bal := e.state.GetEnergy(addr, blockTime)
	if amount.Sign() != 0 {
		if bal.Cmp(amount) < 0 {
			return false
		}

		var total totalAddSub
		e.getStorage(totalAddSubKey, &total)
		e.setStorage(totalAddSubKey, &totalAddSub{
			TotalAdd: total.TotalAdd,
			TotalSub: new(big.Int).Add(total.TotalSub, amount),
		})

		e.state.SetEnergy(addr, new(big.Int).Sub(bal, amount), blockTime)
	} else {
		e.state.SetEnergy(addr, bal, blockTime)
	}
	return true
}

package energy

import (
	"math/big"

	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var (
	tokenSupplyKey = thor.Blake2b([]byte("token-supply"))
	totalAddKey    = thor.Blake2b([]byte("total-add"))
	totalSubKey    = thor.Blake2b([]byte("total-sub"))
)

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
	e.state.GetStructedStorage(e.addr, key, val)
}

func (e *Energy) setStorage(key thor.Bytes32, val interface{}) {
	e.state.SetStructedStorage(e.addr, key, val)
}

// InitializeTokenSupply initializes token supply, to help calculating total energy supply.
func (e *Energy) InitializeTokenSupply(supply *big.Int) {
	e.setStorage(tokenSupplyKey, supply)
}

// GetTotalSupply returns total supply of energy.
func (e *Energy) GetTotalSupply(blockNum uint32) *big.Int {
	// that's totalGrown + totalAdd - totalSub
	var tokenSupply big.Int
	e.getStorage(tokenSupplyKey, &tokenSupply)

	// calc grown energy for total token supply
	energyState := state.EnergyState{Energy: &big.Int{}}
	return energyState.CalcEnergy(&tokenSupply, blockNum)
}

// GetTotalBurned returns energy totally burned.
func (e *Energy) GetTotalBurned() *big.Int {
	var totalAdd, totalSub big.Int
	e.getStorage(totalAddKey, &totalAdd)
	e.getStorage(totalSubKey, &totalSub)
	return new(big.Int).Sub(&totalSub, &totalAdd)
}

// GetBalance returns energy balance of an account at given block time.
func (e *Energy) GetBalance(addr thor.Address, blockNum uint32) *big.Int {
	return e.state.GetEnergy(addr, blockNum)
}

func (e *Energy) AddBalance(addr thor.Address, amount *big.Int, blockNum uint32) {
	bal := e.state.GetEnergy(addr, blockNum)
	if amount.Sign() != 0 {
		var totalAdd big.Int
		e.getStorage(totalAddKey, &totalAdd)
		e.setStorage(totalAddKey, totalAdd.Add(&totalAdd, amount))

		e.state.SetEnergy(addr, new(big.Int).Add(bal, amount), blockNum)
	} else {
		e.state.SetEnergy(addr, bal, blockNum)
	}
}

func (e *Energy) SubBalance(addr thor.Address, amount *big.Int, blockNum uint32) bool {
	bal := e.state.GetEnergy(addr, blockNum)
	if amount.Sign() != 0 {
		if bal.Cmp(amount) < 0 {
			return false
		}
		var totalSub big.Int
		e.getStorage(totalSubKey, &totalSub)
		e.setStorage(totalSubKey, totalSub.Add(&totalSub, amount))

		e.state.SetEnergy(addr, new(big.Int).Sub(bal, amount), blockNum)
	} else {
		e.state.SetEnergy(addr, bal, blockNum)
	}
	return true
}

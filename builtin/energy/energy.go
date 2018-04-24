package energy

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var (
	tokenSupplyKey = thor.Blake2b([]byte("token-supply"))
	totalAddSubKey = thor.Blake2b([]byte("total-add-sub"))
)

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
	var total totalAddSub
	e.getStorage(totalAddSubKey, &total)
	return new(big.Int).Sub(total.TotalSub, total.TotalAdd)
}

// GetBalance returns energy balance of an account at given block time.
func (e *Energy) GetBalance(addr thor.Address, blockNum uint32) *big.Int {
	return e.state.GetEnergy(addr, blockNum)
}

func (e *Energy) AddBalance(addr thor.Address, amount *big.Int, blockNum uint32) {
	bal := e.state.GetEnergy(addr, blockNum)
	if amount.Sign() != 0 {
		var total totalAddSub
		e.getStorage(totalAddSubKey, &total)
		e.setStorage(totalAddSubKey, &totalAddSub{
			TotalAdd: new(big.Int).Add(total.TotalAdd, amount),
			TotalSub: total.TotalSub,
		})

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

		var total totalAddSub
		e.getStorage(totalAddSubKey, &total)
		e.setStorage(totalAddSubKey, &totalAddSub{
			TotalAdd: total.TotalAdd,
			TotalSub: new(big.Int).Add(total.TotalSub, amount),
		})

		e.state.SetEnergy(addr, new(big.Int).Sub(bal, amount), blockNum)
	} else {
		e.state.SetEnergy(addr, bal, blockNum)
	}
	return true
}

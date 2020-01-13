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

func (e *Energy) getInitialSupply() (init initialSupply, err error) {
	err = e.state.DecodeStorage(e.addr, initialSupplyKey, func(raw []byte) error {
		if len(raw) == 0 {
			init = initialSupply{&big.Int{}, &big.Int{}, 0}
			return nil
		}
		return rlp.DecodeBytes(raw, &init)
	})
	return
}

func (e *Energy) getTotalAddSub() (total totalAddSub, err error) {
	err = e.state.DecodeStorage(e.addr, totalAddSubKey, func(raw []byte) error {
		if len(raw) == 0 {
			total = totalAddSub{&big.Int{}, &big.Int{}}
			return nil
		}
		return rlp.DecodeBytes(raw, &total)
	})
	return
}
func (e *Energy) setTotalAddSub(total totalAddSub) error {
	return e.state.EncodeStorage(e.addr, totalAddSubKey, func() ([]byte, error) {
		return rlp.EncodeToBytes(&total)
	})
}

// SetInitialSupply set initial token and energy supply, to help calculating total energy supply.
func (e *Energy) SetInitialSupply(token *big.Int, energy *big.Int) error {
	return e.state.EncodeStorage(e.addr, initialSupplyKey, func() ([]byte, error) {
		return rlp.EncodeToBytes(&initialSupply{
			Token:     token,
			Energy:    energy,
			BlockTime: e.blockTime,
		})
	})
}

// TokenTotalSupply returns total supply of VET.
func (e *Energy) TokenTotalSupply() (*big.Int, error) {
	init, err := e.getInitialSupply()
	if err != nil {
		return nil, err
	}
	return init.Token, nil
}

// TotalSupply returns total supply of energy.
func (e *Energy) TotalSupply() (*big.Int, error) {
	initialSupply, err := e.getInitialSupply()
	if err != nil {
		return nil, err
	}

	// calc grown energy for total token supply
	acc := state.Account{
		Balance:   initialSupply.Token,
		Energy:    initialSupply.Energy,
		BlockTime: initialSupply.BlockTime}
	return acc.CalcEnergy(e.blockTime), nil
}

// TotalBurned returns energy totally burned.
func (e *Energy) TotalBurned() (*big.Int, error) {
	total, err := e.getTotalAddSub()
	if err != nil {
		return nil, err
	}
	return new(big.Int).Sub(total.TotalSub, total.TotalAdd), nil
}

// Get returns energy of an account at given block time.
func (e *Energy) Get(addr thor.Address) (*big.Int, error) {
	return e.state.GetEnergy(addr, e.blockTime)
}

// Add add amount of energy to given address.
func (e *Energy) Add(addr thor.Address, amount *big.Int) error {
	if amount.Sign() == 0 {
		return nil
	}
	eng, err := e.state.GetEnergy(addr, e.blockTime)
	if err != nil {
		return err
	}

	total, err := e.getTotalAddSub()
	if err != nil {
		return err
	}
	total.TotalAdd = new(big.Int).Add(total.TotalAdd, amount)
	if err := e.setTotalAddSub(total); err != nil {
		return err
	}

	return e.state.SetEnergy(addr, new(big.Int).Add(eng, amount), e.blockTime)
}

// Sub sub amount of energy from given address.
// False is returned if no enough energy.
func (e *Energy) Sub(addr thor.Address, amount *big.Int) (bool, error) {
	if amount.Sign() == 0 {
		return true, nil
	}
	eng, err := e.state.GetEnergy(addr, e.blockTime)
	if err != nil {
		return false, err
	}
	if eng.Cmp(amount) < 0 {
		return false, nil
	}
	total, err := e.getTotalAddSub()
	if err != nil {
		return false, err
	}
	total.TotalSub = new(big.Int).Add(total.TotalSub, amount)
	if err := e.setTotalAddSub(total); err != nil {
		return false, err
	}

	if err := e.state.SetEnergy(addr, new(big.Int).Sub(eng, amount), e.blockTime); err != nil {
		return false, err
	}
	return true, nil
}

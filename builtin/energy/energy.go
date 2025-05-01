// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package energy

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/builtin/params"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

var (
	initialSupplyKey = thor.Blake2b([]byte("initial-supply"))
	totalAddSubKey   = thor.Blake2b([]byte("total-add-sub"))
	issuedKey        = thor.Blake2b([]byte("issued"))
)

// Energy implements energy operations.
type Energy struct {
	addr      thor.Address
	state     *state.State
	blockTime uint64
	params    *params.Params
}

// New creates a new energy instance.
func New(addr thor.Address, state *state.State, blockTime uint64, params *params.Params) *Energy {
	return &Energy{addr, state, blockTime, params}
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

func (e *Energy) getIssued() (issued *big.Int, err error) {
	err = e.state.DecodeStorage(e.addr, issuedKey, func(raw []byte) error {
		if len(raw) == 0 {
			issued = big.NewInt(0)
			return nil
		}
		return rlp.DecodeBytes(raw, &issued)
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

	hayabusaForkTime, err := e.state.GetHayabusaForkTime()

	if err != nil {
		return nil, err
	}
	preHayabusa := acc.CalcEnergy(e.blockTime, *hayabusaForkTime)
	postHayabusa, err := e.getIssued()
	if err != nil {
		return nil, err
	}

	return big.NewInt(0).Add(preHayabusa, postHayabusa), nil
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

func (e *Energy) StopEnergyGrowth() {
	bt := big.NewInt(int64(e.blockTime))
	e.state.SetStorage(thor.BytesToAddress([]byte("Energy")), thor.HayabusaEnergyGrowthStopTime, thor.BytesToBytes32(bt.Bytes()))
}

func (e *Energy) addIssued(issued *big.Int) error {
	total, err := e.getIssued()
	if err != nil {
		return err
	}
	total.Add(total, issued)
	return e.state.EncodeStorage(e.addr, issuedKey, func() ([]byte, error) {
		return rlp.EncodeToBytes(&total)
	})
}

type staker interface {
	LockedVET() (*big.Int, error)
	GetDelegationLockedVET(validationID thor.Bytes32) (*big.Int, error)
}

func (e *Energy) DistributeRewards(validationID thor.Bytes32, beneficiary thor.Address, staker staker) error {
	reward, err := e.CalculateRewards(staker)
	if err != nil {
		return err
	}

	delegatedVET, err := staker.GetDelegationLockedVET(validationID)
	if err != nil {
		return err
	}

	// If delegated amount of VET is 0 then transfer the whole reward to the validator
	proposerReward := new(big.Int).Set(reward)
	if delegatedVET.Cmp(big.NewInt(0)) != 0 {
		proposerReward.Mul(proposerReward, big.NewInt(3))
		proposerReward.Div(proposerReward, big.NewInt(10))

		val, err := e.params.Get(thor.KeyStargateContractAddress)
		if err != nil {
			return err
		}

		addr := thor.BytesToAddress(val.Bytes())
		addrEng, err := e.state.GetEnergy(addr, e.blockTime)
		if err != nil {
			return err
		}
		if err := e.state.SetEnergy(addr, new(big.Int).Add(addrEng, big.NewInt(0).Sub(reward, proposerReward)), e.blockTime); err != nil {
			return err
		}
	}
	beneficiaryEng, err := e.state.GetEnergy(beneficiary, e.blockTime)
	if err != nil {
		return err
	}

	// we don't use e.add which is adding to total add sub since that function is only meant to be used for transactions
	// which are also burning the amount, distribute rewards is used only for distribution so in this case we just set the
	// energy and increase the issued to be able to keep track of totalSupply
	if err := e.state.SetEnergy(beneficiary, new(big.Int).Add(beneficiaryEng, proposerReward), e.blockTime); err != nil {
		return err
	}
	if err := e.addIssued(reward); err != nil {
		return err
	}
	return nil
}

func (e *Energy) CalculateRewards(staker staker) (*big.Int, error) {
	totalStaked, err := staker.LockedVET()
	if err != nil {
		return nil, err
	}
	bigE18 := big.NewInt(1e18)
	// sqrt(totalStaked / 1e18) * 1e18, we are calculating sqrt on VET and then converting to wei
	sqrtStake := new(big.Int).Sqrt(new(big.Int).Div(totalStaked, bigE18))
	sqrtStake.Mul(sqrtStake, bigE18)

	currentYear := time.Now().Year()
	isLeap := isLeapYear(currentYear)
	blocksPerYear := thor.NumberOfBlocksPerYear
	if isLeap {
		blocksPerYear = new(big.Int).Sub(thor.NumberOfBlocksPerYear, big.NewInt(thor.SeederInterval))
	}

	// reward = 1 * TargetFactor * ScalingFactor * sqrt(totalStaked / 1e18) / blocksPerYear
	reward := big.NewInt(1)
	reward.Mul(reward, thor.TargetFactor)
	reward.Mul(reward, thor.ScalingFactor)
	reward.Mul(reward, sqrtStake)
	reward.Div(reward, blocksPerYear)

	return reward, nil
}

func isLeapYear(year int) bool {
	if year%4 == 0 {
		if year%100 == 0 {
			return year%400 == 0
		}
		return true
	}
	return false
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package energy

import (
	"errors"
	"io"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/builtin/params"

	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

var (
	initialSupplyKey  = thor.Blake2b([]byte("initial-supply"))
	totalAddSubKey    = thor.Blake2b([]byte("total-add-sub"))
	issuedKey         = thor.Blake2b([]byte("issued"))
	growthStopTimeKey = thor.Blake2b([]byte("growth-stop-time"))
	bigE18            = big.NewInt(1e18)
)

type StopTimeFunc func() (uint64, error)

// Energy implements energy operations.
type Energy struct {
	addr      thor.Address
	state     *state.State
	blockTime uint64
	stopTime  StopTimeFunc
	params    *params.Params
}

// New creates a new energy instance.
func New(addr thor.Address, state *state.State, blockTime uint64, stopTime StopTimeFunc, params *params.Params) *Energy {
	return &Energy{
		addr:      addr,
		state:     state,
		blockTime: blockTime,
		params:    params,
		stopTime:  stopTime,
	}
}

func (e *Energy) StopTimeFunc() StopTimeFunc {
	return e.stopTime
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

// TotalSupply returns total supply of energy. Does not account for burned tokens.
func (e *Energy) TotalSupply() (*big.Int, error) {
	initialSupply, err := e.getInitialSupply()
	if err != nil {
		return nil, err
	}

	// calc grown energy for total token supply
	acc := state.Account{
		Balance:   initialSupply.Token,
		Energy:    initialSupply.Energy,
		BlockTime: initialSupply.BlockTime,
	}

	// this is a virtual account, use account.CalcEnergy directly
	stopTime, err := e.stopTime()
	if err != nil {
		return nil, err
	}
	grown := acc.CalcEnergy(e.blockTime, stopTime)

	issued, err := e.getIssued()
	if err != nil {
		return nil, err
	}

	return grown.Add(grown, issued), nil
}

// TotalBurned returns energy totally burned.
func (e *Energy) TotalBurned() (*big.Int, error) {
	total, err := e.getTotalAddSub()
	if err != nil {
		return nil, err
	}
	burned := new(big.Int).Sub(total.TotalSub, total.TotalAdd)
	return burned, nil
}

// Get returns energy of an account at given block time.
func (e *Energy) Get(addr thor.Address) (*big.Int, error) {
	stopTime, err := e.stopTime()
	if err != nil {
		return nil, err
	}

	return e.state.GetEnergy(addr, e.blockTime, stopTime)
}

// Add add amount of energy to given address.
func (e *Energy) Add(addr thor.Address, amount *big.Int) error {
	if amount.Sign() == 0 {
		return nil
	}
	eng, err := e.Get(addr)
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
	eng, err := e.Get(addr)
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

// StopEnergyGrowth sets the end time of energy growth at the current block time.
func (e *Energy) StopEnergyGrowth() error {
	if ts, err := e.GetEnergyGrowthStopTime(); err != nil {
		return err
	} else if ts != math.MaxUint64 {
		// We simply ignore multiple calls to this function
		return nil
	}

	if err := e.state.EncodeStorage(e.addr, growthStopTimeKey, func() ([]byte, error) {
		return rlp.EncodeToBytes(e.blockTime)
	}); err != nil {
		return err
	}

	return nil
}

// GetEnergyGrowthStopTime returns the stop time of energy growth
// if the stop time is not set, return math.MaxUint64
func (e *Energy) GetEnergyGrowthStopTime() (uint64, error) {
	var time uint64 = math.MaxUint64
	if err := e.state.DecodeStorage(e.addr, growthStopTimeKey, func(raw []byte) error {
		if len(raw) == 0 {
			return nil
		}
		var err error
		time, err = decodeUint64(raw)
		return err
	}); err != nil {
		return math.MaxUint64, err
	}

	return time, nil
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
	LockedStake() (uint64, uint64, error)
	HasDelegations(address thor.Address) (bool, error)
	IncreaseDelegatorsReward(master thor.Address, reward *big.Int, currentBlock uint32) error
}

func (e *Energy) DistributeRewards(beneficiary, signer thor.Address, staker staker, currentBlock uint32) error {
	reward, err := e.CalculateRewards(staker)
	if err != nil {
		return err
	}
	hasDelegations, err := staker.HasDelegations(signer)
	if err != nil {
		return err
	}
	validatorRewardPerc, err := e.params.Get(thor.KeyValidatorRewardPercentage)
	if err != nil {
		return err
	}
	if validatorRewardPerc.Uint64() == 0 {
		validatorRewardPerc = big.NewInt(int64(thor.InitialValidatorRewardPercentage))
	}

	// If delegated amount of VET is 0 then transfer the whole reward to the validator
	proposerReward := new(big.Int).Set(reward)
	if hasDelegations && validatorRewardPerc.Cmp(big.NewInt(100)) < 0 {
		proposerReward.Mul(proposerReward, validatorRewardPerc)
		proposerReward.Div(proposerReward, big.NewInt(100))

		val, err := e.params.Get(thor.KeyDelegatorContractAddress)
		if err != nil {
			return err
		}

		addr := thor.BytesToAddress(val.Bytes())
		addrEng, err := e.Get(addr)
		if err != nil {
			return err
		}
		delegationReward := new(big.Int).Sub(reward, proposerReward)
		if err := staker.IncreaseDelegatorsReward(signer, delegationReward, currentBlock); err != nil {
			return err
		}
		if err := e.state.SetEnergy(addr, new(big.Int).Add(addrEng, delegationReward), e.blockTime); err != nil {
			return err
		}
	}
	beneficiaryEng, err := e.Get(beneficiary)
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
	totalStaked, _, err := staker.LockedStake()
	if err != nil {
		return nil, err
	}
	// sqrt(totalStaked in VET) * 1e18, we are calculating sqrt on VET and then converting to wei
	sqrtStake := new(big.Int).Sqrt(new(big.Int).SetUint64(totalStaked))
	sqrtStake.Mul(sqrtStake, bigE18)

	curveFactor, err := e.params.Get(thor.KeyCurveFactor)
	if err != nil {
		return nil, err
	}
	if curveFactor.Uint64() == 0 {
		curveFactor = thor.InitialCurveFactor
	}

	// reward = 1 * curveFactor * sqrt(totalStaked / 1e18) / blocksPerYear
	reward := big.NewInt(1)
	reward.Mul(reward, curveFactor)
	reward.Mul(reward, sqrtStake)
	reward.Div(reward, thor.NumberOfBlocksPerYear)
	return reward, nil
}

// decodeUint64 efficiently decodes RLP data to uint64 without reflection
func decodeUint64(encoded []byte) (uint64, error) {
	if len(encoded) == 0 {
		return 0, io.EOF
	}

	b := encoded[0] // kind byte
	switch {
	case b < 0x80: // type bytes [0x00, 0x7F]
		return uint64(b), nil
	case b < 0xB8: // type string 0-55 bytes
		size := uint64(b - 0x80)
		if size > 8 {
			return 0, errors.New("rlp: uint overflow")
		}
		if len(encoded) < int(1+size) {
			return 0, errors.New("rlp: unexpected EOF")
		}
		switch size {
		case 0:
			return 0, nil
		case 1:
			return uint64(encoded[1]), nil
		default:
			var decoded uint64
			for _, b := range encoded[1 : 1+size] {
				decoded = (decoded << 8) | uint64(b)
			}
			return decoded, nil
		}
	case b < 0xC0: // type string > 55 bytes
		return 0, errors.New("rlp: uint overflow")
	default:
		return 0, errors.New("rlp: expected string")
	}
}

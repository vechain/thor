// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package staker

import (
	"fmt"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"math/big"
)

var (
	stakesKey     = thor.Blake2b([]byte("_stakes"))
	validatorsKey = thor.Blake2b([]byte("_validators"))
	minimumStake  = big.NewInt(0).Mul(big.NewInt(25e6), big.NewInt(1e18)) // 25M VET
)

// Staker implements staker operations.
type Staker struct {
	addr      thor.Address
	state     *state.State
	blockTime uint64
}

type Stake map[thor.Address]*big.Int

type StakeRecord struct {
	Address thor.Address
	Amount  *big.Int
}

type ValidatorStakes map[thor.Address]Stake

type ValidatorStakeRecord struct {
	Validator thor.Address
	Stakes    []StakeRecord
}

// New creates a new staker instance.
func New(addr thor.Address, state *state.State, blockTime uint64) *Staker {
	return &Staker{addr, state, blockTime}
}

func rlcEncodeMapping(stakes map[thor.Address]map[thor.Address]*big.Int) ([]byte, error) {
	var stakeList []ValidatorStakeRecord
	for addr, stake := range stakes {
		var validatorStakeList []StakeRecord
		for stakerAddr, amount := range stake {
			validatorStakeList = append(validatorStakeList, StakeRecord{stakerAddr, amount})
		}
		stakeList = append(stakeList, ValidatorStakeRecord{addr, validatorStakeList})
	}
	return rlp.EncodeToBytes(stakeList)
}

func (b *Staker) getStakesForValidator(addr thor.Address) (stakes map[thor.Address]*big.Int, err error) {
	err = b.state.DecodeStorage(b.addr, stakesKey, func(raw []byte) error {
		if len(raw) == 0 {
			stakes = make(map[thor.Address]*big.Int)
			return nil
		}
		var stakeList []ValidatorStakeRecord
		err := rlp.DecodeBytes(raw, &stakeList)
		if err != nil {
			return err
		}
		for _, stakeRecord := range stakeList {
			if stakeRecord.Validator == addr {
				stakes = make(map[thor.Address]*big.Int)
				for _, record := range stakeRecord.Stakes {
					stakes[record.Address] = record.Amount
				}
				return nil
			}
		}
		return nil
	})
	return stakes, nil
}

func (b *Staker) getStakes() (stakes map[thor.Address]map[thor.Address]*big.Int, err error) {
	stakes = make(map[thor.Address]map[thor.Address]*big.Int)
	err = b.state.DecodeStorage(b.addr, stakesKey, func(raw []byte) error {
		if len(raw) == 0 {
			return nil
		}
		var stakeList []ValidatorStakeRecord
		err := rlp.DecodeBytes(raw, &stakeList)
		if err != nil {
			return err
		}
		for _, stakeRecord := range stakeList {
			stake := make(map[thor.Address]*big.Int)
			for _, record := range stakeRecord.Stakes {
				stake[record.Address] = record.Amount
			}
			stakes[stakeRecord.Validator] = stake
		}
		return nil
	})
	return stakes, nil
}

func (b *Staker) TotalStake() (totalStaked *big.Int, err error) {
	return b.state.GetBalance(b.addr)
}

func (b *Staker) GetStake(addr thor.Address, validatorAddr thor.Address) (stake *big.Int, err error) {
	stakes, err := b.getStakesForValidator(validatorAddr)
	if err != nil {
		return &big.Int{}, err
	}
	if stake, ok := stakes[addr]; ok {
		return stake, nil
	}
	return &big.Int{}, nil
}

func (b *Staker) Stake(addr thor.Address, validatorAddr thor.Address, amount *big.Int) error {
	if amount.Sign() == 0 {
		return nil
	}

	var validatorExists bool
	if validators, err := b.ListValidators(); err != nil {
		return err
	} else if len(validators) > 0 {
		for _, validator := range validators {
			if validator == validatorAddr {
				validatorExists = true
				break
			}
		}
	}

	if !validatorExists {
		return fmt.Errorf("validator %s does not exist", validatorAddr.String())
	}

	return b.stake(addr, validatorAddr, amount)
}

func (b *Staker) stake(addr thor.Address, validatorAddr thor.Address, amount *big.Int) error {
	err := b.state.EncodeStorage(b.addr, stakesKey, func() ([]byte, error) {
		stakes, stakesError := b.getStakesForValidator(validatorAddr)
		if stakesError != nil {
			return nil, stakesError
		}
		if stakes == nil {
			stakes = make(map[thor.Address]*big.Int)
		}
		var currentStake, exists = stakes[addr]
		if !exists {
			currentStake = &big.Int{}
		}
		stakes[addr] = currentStake.Add(currentStake, amount)
		validatorStakes, err := b.getStakes()
		if err != nil {
			return nil, err
		}
		validatorStakes[validatorAddr] = stakes
		return rlcEncodeMapping(validatorStakes)
	})
	return err
}

func (b *Staker) Unstake(addr thor.Address, amount *big.Int, validatorAddr thor.Address) error {
	if addr == validatorAddr {
		return fmt.Errorf("validator cannot unstake from itself")
	}
	return b.unstake(addr, amount, validatorAddr)
}

func (b *Staker) unstake(addr thor.Address, amount *big.Int, validatorAddr thor.Address) error {
	if amount.Sign() == 0 {
		return nil
	}
	totalStaked, err := b.TotalStake()
	if err != nil {
		return err
	}

	accountStaked, err := b.GetStake(addr, validatorAddr)
	if err != nil {
		return err
	}
	contractBalance, err := b.state.GetBalance(b.addr)
	if err != nil {
		return err
	}
	if totalStaked.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient total staked: total staked %s is less than amount %s", totalStaked.String(), amount.String())
	}
	if contractBalance.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient balance: contract balance %s is less than amount %s", contractBalance.String(), amount.String())
	}
	if accountStaked.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient stake: account stake %s is less than amount %s", accountStaked.String(), amount.String())
	}

	accountBalance, err := b.state.GetBalance(addr)
	if err != nil {
		return err
	}
	err = b.state.SetBalance(b.addr, contractBalance.Sub(contractBalance, amount))
	if err != nil {
		return err
	}

	err = b.state.SetBalance(addr, accountBalance.Add(accountBalance, amount))
	if err != nil {
		return err
	}

	err = b.state.EncodeStorage(b.addr, stakesKey, func() ([]byte, error) {
		stakes, stakesError := b.getStakesForValidator(validatorAddr)
		if stakesError != nil {
			return nil, stakesError
		}
		var currentStake = stakes[addr]
		if currentStake == nil {
			currentStake = &big.Int{}
			stakes[addr] = currentStake
		}
		stakes[addr] = currentStake.Sub(stakes[addr], amount)
		validatorStakes, err := b.getStakes()
		if err != nil {
			return nil, err
		}
		if validatorStakes == nil {
			fmt.Errorf("validatorStakes is nil")
		}
		validatorStakes[validatorAddr] = stakes
		return rlcEncodeMapping(validatorStakes)
	})
	return err
}

func (b *Staker) AddValidator(amount *big.Int, addr thor.Address) error {
	var validators []thor.Address
	if amount.Cmp(minimumStake) < 0 {
		return fmt.Errorf("amount is less than minimum stake")
	}
	err := b.state.DecodeStorage(b.addr, validatorsKey, func(raw []byte) error {
		if len(raw) == 0 {
			validators = []thor.Address{}
			return nil
		}
		return rlp.DecodeBytes(raw, &validators)
	})
	if err != nil {
		return err
	}

	if len(validators) == 0 {
		validators = []thor.Address{addr}
	} else {
		for _, validator := range validators {
			if validator == addr {
				return fmt.Errorf("validator already exists")
			}
		}
		validators = append(validators, addr)
	}

	err = b.stake(addr, addr, amount)
	if err != nil {
		return err
	}
	// Encode the updated list back to storage
	return b.state.EncodeStorage(b.addr, validatorsKey, func() ([]byte, error) {
		return rlp.EncodeToBytes(validators)
	})
}

func (b *Staker) ListValidators() ([]thor.Address, error) {
	var validators []thor.Address
	err := b.state.DecodeStorage(b.addr, validatorsKey, func(raw []byte) error {
		if len(raw) == 0 {
			validators = []thor.Address{}
			return nil
		}
		return rlp.DecodeBytes(raw, &validators)
	})
	return validators, err
}

func (b *Staker) RemoveValidator(addr thor.Address) error {
	var validators []thor.Address
	err := b.state.DecodeStorage(b.addr, validatorsKey, func(raw []byte) error {
		if len(raw) == 0 {
			validators = []thor.Address{}
			return nil
		}
		return rlp.DecodeBytes(raw, &validators)
	})
	if err != nil {
		return err
	}

	var validatorExists = false
	for i, validator := range validators {
		if validator == addr {
			validators = append(validators[:i], validators[i+1:]...)
			validatorExists = true
			break
		}
	}

	if validatorExists {
		stakes, err := b.getStakesForValidator(addr)
		if err != nil {
			return err
		}
		for stakerAddr, amount := range stakes {
			err = b.unstake(stakerAddr, amount, addr)
			if err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("validator does not exist")
	}

	// Encode the updated list back to storage
	return b.state.EncodeStorage(b.addr, validatorsKey, func() ([]byte, error) {
		return rlp.EncodeToBytes(validators)
	})
}

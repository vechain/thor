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
	totalStakedKey = thor.Blake2b([]byte("_totalStaked"))
	stakesKey      = thor.Blake2b([]byte("_stakes"))
)

// Staker implements staker operations.
type Staker struct {
	addr      thor.Address
	state     *state.State
	blockTime uint64
}

type Stake struct {
	Address thor.Address
	Amount  *big.Int
}

// New creates a new staker instance.
func New(addr thor.Address, state *state.State, blockTime uint64) *Staker {
	return &Staker{addr, state, blockTime}
}

func rlcEncodeMapping(stakes map[thor.Address]*big.Int) ([]byte, error) {
	var stakeList []Stake
	for addr, amount := range stakes {
		stakeList = append(stakeList, Stake{Address: addr, Amount: amount})
	}
	return rlp.EncodeToBytes(stakeList)
}

func (b *Staker) getStakes() (stakes map[thor.Address]*big.Int, err error) {
	err = b.state.DecodeStorage(b.addr, stakesKey, func(raw []byte) error {
		if len(raw) == 0 {
			stakes = make(map[thor.Address]*big.Int)
			return nil
		}
		var stakeList []Stake
		err := rlp.DecodeBytes(raw, &stakeList)
		if err != nil {
			return err
		}
		stakes = make(map[thor.Address]*big.Int)
		for _, stake := range stakeList {
			stakes[stake.Address] = stake.Amount
		}
		return nil
	})
	return
}

func (b *Staker) TotalStake() (totalStaked *big.Int, err error) {
	err = b.state.DecodeStorage(b.addr, totalStakedKey, func(raw []byte) error {
		if len(raw) == 0 {
			totalStaked = &big.Int{}
			return nil
		}
		return rlp.DecodeBytes(raw, &totalStaked)
	})
	return
}

func (b *Staker) GetStake(addr thor.Address) (stake *big.Int, err error) {
	stakes, err := b.getStakes()
	if err != nil {
		return &big.Int{}, err
	}
	if stake, ok := stakes[addr]; ok {
		return stake, nil
	}
	return &big.Int{}, nil
}

func (b *Staker) Stake(addr thor.Address, amount *big.Int) error {
	if amount.Sign() == 0 {
		return nil
	}
	addressBalance, err := b.state.GetBalance(addr)
	if err != nil {
		panic(err)
	}
	if addressBalance.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient balance: address balance %s is less than amount %s", addressBalance.String(), amount.String())
	}

	totalStaked, err := b.TotalStake()
	if err != nil {
		return err
	}
	err = b.state.EncodeStorage(b.addr, totalStakedKey, func() ([]byte, error) {
		return rlp.EncodeToBytes(totalStaked.Add(totalStaked, amount))
	})

	err = b.state.EncodeStorage(b.addr, stakesKey, func() ([]byte, error) {
		stakes, stakesError := b.getStakes()
		if stakesError != nil {
			return nil, stakesError
		}
		var currentStake = stakes[addr]
		if currentStake == nil {
			currentStake = &big.Int{}
			stakes[addr] = currentStake
		}
		stakes[addr] = currentStake.Add(stakes[addr], amount)
		return rlcEncodeMapping(stakes)
	})
	return err
}

func (b *Staker) Unstake(addr thor.Address, amount *big.Int) error {
	if amount.Sign() == 0 {
		return nil
	}
	totalStaked, err := b.TotalStake()
	if err != nil {
		panic(err)
	}

	accountStaked, err := b.GetStake(addr)
	if err != nil {
		panic(err)
	}
	contractBalance, err := b.state.GetBalance(b.addr)
	if err != nil {
		panic(err)
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
		panic(err)
	}
	err = b.state.SetBalance(b.addr, contractBalance.Sub(contractBalance, amount))
	if err != nil {
		panic(err)
	}

	err = b.state.SetBalance(addr, accountBalance.Add(accountBalance, amount))
	if err != nil {
		panic(err)
	}

	err = b.state.EncodeStorage(b.addr, totalStakedKey, func() ([]byte, error) {
		return rlp.EncodeToBytes(totalStaked.Sub(totalStaked, amount))
	})
	err = b.state.EncodeStorage(b.addr, stakesKey, func() ([]byte, error) {
		stakes, stakesError := b.getStakes()
		if stakesError != nil {
			return nil, stakesError
		}
		var currentStake = stakes[addr]
		if currentStake == nil {
			currentStake = &big.Int{}
			stakes[addr] = currentStake
		}
		stakes[addr] = currentStake.Sub(stakes[addr], amount)
		return rlcEncodeMapping(stakes)
	})
	return err
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package prototype

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/state"
)

type userPlan struct {
	Credit       *big.Int
	RecoveryRate *big.Int
}

var (
	_ state.StorageEncoder = (*userPlan)(nil)
	_ state.StorageDecoder = (*userPlan)(nil)
)

func (up *userPlan) Encode() ([]byte, error) {
	if up.Credit.Sign() == 0 &&
		up.RecoveryRate.Sign() == 0 {
		return nil, nil
	}
	return rlp.EncodeToBytes(up)
}

func (up *userPlan) Decode(data []byte) error {
	if len(data) == 0 {
		*up = userPlan{&big.Int{}, &big.Int{}}
		return nil
	}
	return rlp.DecodeBytes(data, up)
}

type userObject struct {
	RemainedCredit *big.Int
	BlockTime      uint64
}

var (
	_ state.StorageEncoder = (*userObject)(nil)
	_ state.StorageDecoder = (*userObject)(nil)
)

func (u *userObject) Encode() ([]byte, error) {
	if u.IsEmpty() {
		return nil, nil
	}
	return rlp.EncodeToBytes(u)
}

func (u *userObject) Decode(data []byte) error {
	if len(data) == 0 {
		*u = userObject{&big.Int{}, 0}
		return nil
	}
	return rlp.DecodeBytes(data, u)
}

func (u *userObject) IsEmpty() bool {
	return u.RemainedCredit.Sign() == 0 && u.BlockTime == 0
}

func (u *userObject) Credit(plan *userPlan, blockTime uint64) *big.Int {
	x := new(big.Int).SetUint64(blockTime - u.BlockTime)
	x.Mul(x, plan.RecoveryRate)
	x.Add(x, u.RemainedCredit)
	if x.Cmp(plan.Credit) < 0 {
		return x
	}
	return plan.Credit
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package prototype

import (
	"math/big"
)

type creditPlan struct {
	Credit       *big.Int
	RecoveryRate *big.Int
}

func (u *creditPlan) IsEmpty() bool {
	return u.Credit.Sign() == 0 && u.RecoveryRate.Sign() == 0
}

type userObject struct {
	UsedCredit *big.Int
	BlockTime  uint64
}

func (u *userObject) IsEmpty() bool {
	return u.UsedCredit.Sign() == 0 && u.BlockTime == 0
}

func (u *userObject) Credit(plan *creditPlan, blockTime uint64) *big.Int {
	if u.UsedCredit.Sign() == 0 {
		return plan.Credit
	}

	var used *big.Int
	if blockTime <= u.BlockTime {
		used = u.UsedCredit
	} else {
		x := new(big.Int).SetUint64(blockTime - u.BlockTime)
		x.Mul(x, plan.RecoveryRate)
		used = x.Sub(u.UsedCredit, x)
	}

	if used.Sign() <= 0 {
		return plan.Credit
	}

	if used.Cmp(plan.Credit) >= 0 {
		return &big.Int{}
	}
	return new(big.Int).Sub(plan.Credit, used)
}

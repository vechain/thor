// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package energy

import (
	"math/big"
)

type (
	initialSupply struct {
		Token     *big.Int
		Energy    *big.Int
		BlockTime uint64
	}
	totalAddSub struct {
		TotalAdd *big.Int
		TotalSub *big.Int
	}
)

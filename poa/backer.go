// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/thor"
)

// EvaluateVRF evalutes if the VRF output(beta) meets the requirement of backer.
func EvaluateVRF(beta []byte, maxBlockProposers uint64) bool {
	// calc the threshold = CommitteMemberSize * MaxBig256 / maxBlockProposers
	x := new(big.Int).SetUint64(thor.CommitteMemberSize)
	x = x.Mul(x, math.MaxBig256)
	x = x.Div(x, new(big.Int).SetUint64(maxBlockProposers))

	// beta <= threshold, then it's a valid beta
	return new(big.Int).SetBytes(beta).Cmp(x) <= 0
}

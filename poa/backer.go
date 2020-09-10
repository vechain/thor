// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package poa

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/vechain/thor/thor"
)

var threshold = new(big.Int).Div(new(big.Int).Mul(math.MaxBig256, big.NewInt(thor.ElectionThreshold)), big.NewInt(100))

// EvaluateVRF evalutes if the VRF output(beta) meets the requirement of backer.
func EvaluateVRF(beta []byte) bool {
	if c := bytes.Compare(beta, threshold.Bytes()); c <= 0 {
		return true
	}
	return false
}

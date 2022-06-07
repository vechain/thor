// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package vrf

import (
	"crypto/elliptic"
	"math/big"

	a "github.com/decred/dcrd/dcrec/secp256k1/v4"
	b "github.com/ethereum/go-ethereum/crypto/secp256k1"
)

// mergedCurve merges fast parts of two secp256k1 curve implementations.
type mergedCurve struct{}

// Params returns the parameters for the curve.
func (c *mergedCurve) Params() *elliptic.CurveParams {
	return a.S256().Params()
}

// IsOnCurve reports whether the given (x,y) lies on the curve.
func (c *mergedCurve) IsOnCurve(x, y *big.Int) bool {
	return a.S256().IsOnCurve(x, y)
}

// Add returns the sum of (x1,y1) and (x2,y2)
func (c *mergedCurve) Add(x1, y1, x2, y2 *big.Int) (x, y *big.Int) {
	return a.S256().Add(x1, y1, x2, y2)
}

// Double returns 2*(x,y)
func (c *mergedCurve) Double(x1, y1 *big.Int) (x, y *big.Int) {
	return a.S256().Double(x1, y1)
}

// ScalarMult returns k*(Bx,By) where k is a number in big-endian form.
func (c *mergedCurve) ScalarMult(x1, y1 *big.Int, k []byte) (x, y *big.Int) {
	return b.S256().ScalarMult(x1, y1, k)
}

// ScalarBaseMult returns k*G, where G is the base point of the group
// and k is an integer in big-endian form.
func (c *mergedCurve) ScalarBaseMult(k []byte) (x, y *big.Int) {
	return b.S256().ScalarBaseMult(k)
}

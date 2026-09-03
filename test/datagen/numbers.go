// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package datagen

import (
	"crypto/rand"
	"math/big"
	mathrand "math/rand/v2"
)

func RandInt() int {
	return mathrand.Int() //#nosec G404
}

func RandUint32() uint32 {
	return mathrand.Uint32() //#nosec G404
}

func RandUint64() uint64 {
	return mathrand.Uint64() //#nosec G404
}

func RandIntN(n int) int {
	return mathrand.N(n) //#nosec G404
}

func RandBigInt() *big.Int {
	var data [32]byte

	rand.Read(data[:])

	return new(big.Int).SetBytes(data[:])
}

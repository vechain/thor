// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBigIntToString(t *testing.T) {
	assert.Equal(t, "<nil>", BigIntToString(nil))

	max256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	for _, v := range []*big.Int{big.NewInt(0), big.NewInt(1), big.NewInt(-12345), max256} {
		assert.Equal(t, v.String(), BigIntToString(v))
	}

	huge := new(big.Int).Lsh(big.NewInt(1), 8*1024*1024)
	s := BigIntToString(huge)
	assert.Less(t, len(s), 200)
	assert.Contains(t, s, "oversized bigint")
}

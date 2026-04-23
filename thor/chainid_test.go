// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/thor"
)

func TestChainID(t *testing.T) {
	mainnetID := thor.MustParseBytes32("0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a")
	testnetID := thor.MustParseBytes32("0x000000000b2bce3c70bc649a02749e8687721b09ed2e15997f466536b20bb127")

	assert.Equal(t, uint64(0x1b4a), thor.ChainID(mainnetID), "mainnet CHAIN_ID = 6986")
	assert.Equal(t, uint64(0xb127), thor.ChainID(testnetID), "testnet CHAIN_ID = 45351")
	assert.Equal(t, uint64(0x0000), thor.ChainID(thor.Bytes32{}), "all-zero genesis -> 0")

	var ff thor.Bytes32
	for i := range ff {
		ff[i] = 0xff
	}
	assert.Equal(t, uint64(0xffff), thor.ChainID(ff), "all-0xff genesis -> 0xffff")
}

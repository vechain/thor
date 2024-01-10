// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
)

func TestTestnetGenesis(t *testing.T) {
	db := muxdb.NewMem()
	gene := genesis.NewTestnet()

	b0, _, _, err := gene.Build(state.NewStater(db))
	assert.Nil(t, err)

	id := gene.ID()
	name := gene.Name()

	assert.Equal(t, id, thor.Bytes32{0x0, 0x0, 0x0, 0x0, 0xb, 0x2b, 0xce, 0x3c, 0x70, 0xbc, 0x64, 0x9a, 0x2, 0x74, 0x9e, 0x86, 0x87, 0x72, 0x1b, 0x9, 0xed, 0x2e, 0x15, 0x99, 0x7f, 0x46, 0x65, 0x36, 0xb2, 0xb, 0xb1, 0x27})
	assert.Equal(t, name, "testnet")

	st := state.New(db, b0.Header().StateRoot(), 0, 0, 0)

	v, err := st.Exists(thor.MustParseAddress("0xe59D475Abe695c7f67a8a2321f33A856B0B4c71d"))
	assert.Nil(t, err)
	assert.True(t, v)
}

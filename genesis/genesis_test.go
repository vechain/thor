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

	assert.Equal(t, id, thor.MustParseBytes32("0x000000000b2bce3c70bc649a02749e8687721b09ed2e15997f466536b20bb127"))
	assert.Equal(t, name, "testnet")

	st := state.New(db, b0.Header().StateRoot(), 0, 0, 0)

	v, err := st.Exists(thor.MustParseAddress("0xe59D475Abe695c7f67a8a2321f33A856B0B4c71d"))
	assert.Nil(t, err)
	assert.True(t, v)
}

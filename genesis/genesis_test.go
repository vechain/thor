// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
)

func TestTestnetGenesis(t *testing.T) {
	kv, _ := lvldb.NewMem()
	gene := genesis.NewTestnet()

	b0, _, err := gene.Build(state.NewCreator(kv))
	assert.Nil(t, err)

	_, err = state.New(b0.Header().StateRoot(), kv)
	assert.Nil(t, err)
}

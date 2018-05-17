// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/genesis"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/state"
)

func TestGenesis(t *testing.T) {
	assert := assert.New(t)
	kv, _ := lvldb.NewMem()
	defer kv.Close()

	g, err := genesis.NewMainnet()
	if err != nil {
		t.Fatal(err)
	}

	b0, _, err := g.Build(state.NewCreator(kv))
	if err != nil {
		t.Fatal(err)
	}

	st, _ := state.New(b0.Header().StateRoot(), kv)
	assert.True(len(st.GetCode(builtin.Authority.Address)) > 0)
}
func TestDevGenesis(t *testing.T) {
	assert := assert.New(t)
	kv, _ := lvldb.NewMem()
	defer kv.Close()
	g, err := genesis.NewDevnet()
	if err != nil {
		t.Fatal(err)
	}
	b0, logs, err := g.Build(state.NewCreator(kv))

	st, _ := state.New(b0.Header().StateRoot(), kv)
	assert.True(len(st.GetCode(builtin.Authority.Address)) > 0)
	fmt.Println(b0, logs)
}

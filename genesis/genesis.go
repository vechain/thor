// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package genesis

import (
	"encoding/hex"

	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

// Genesis to build genesis block.
type Genesis struct {
	builder *Builder
	id      thor.Bytes32
	name    string
}

// Build build the genesis block.
func (g *Genesis) Build(stateCreator *state.Creator) (blk *block.Block, events tx.Events, err error) {
	block, events, err := g.builder.Build(stateCreator)
	if err != nil {
		return nil, nil, err
	}
	if block.Header().ID() != g.id {
		panic("built genesis ID incorrect")
	}
	return block, events, nil
}

// ID returns genesis block ID.
func (g *Genesis) ID() thor.Bytes32 {
	return g.id
}

// Name returns network name.
func (g *Genesis) Name() string {
	return g.name
}

func mustEncodeInput(abi *abi.ABI, name string, args ...interface{}) []byte {
	m, found := abi.MethodByName(name)
	if !found {
		panic("method not found")
	}
	data, err := m.EncodeInput(args...)
	if err != nil {
		panic(err)
	}
	return data
}

func mustDecodeHex(str string) []byte {
	data, err := hex.DecodeString(str)
	if err != nil {
		panic(err)
	}
	return data
}

func mustParseAddress(str string) thor.Address {
	addr, err := thor.ParseAddress(str)
	if err != nil {
		panic(err)
	}
	return addr
}

func mustParseBytes32(str string) thor.Bytes32 {
	b32, err := thor.ParseBytes32(str)
	if err != nil {
		panic(err)
	}
	return b32
}

var emptyRuntimeBytecode = mustDecodeHex("6060604052600256")

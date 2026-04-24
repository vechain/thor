// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/tx"
)

const testChainID uint64 = 0xabcd
const testChainTag byte = 0x01

var testBlockRef = tx.NewBlockRef(0)

func TestBuildMatrix(t *testing.T) {
	m := BuildMatrix()

	// Exact length.
	assert.Len(t, m, 10, "matrix must have 10 entries")

	// All unique by (type, clauses, path).
	seen := map[string]bool{}
	for _, s := range m {
		k := fmt.Sprintf("%d/%d/%s", s.Type, s.Clauses, s.Path)
		assert.False(t, seen[k], "duplicate spec: %s", k)
		seen[k] = true
	}
	assert.Len(t, seen, 10)

	// Exact composition.
	want := map[string]bool{
		"0/1/rest": true, "0/1/rpc": true,
		"0/3/rest": true, "0/3/rpc": true,
		"81/1/rest": true, "81/1/rpc": true, // 0x51 = 81
		"81/3/rest": true, "81/3/rpc": true,
		"2/1/rest": true, "2/1/rpc": true,
	}
	for _, s := range m {
		k := fmt.Sprintf("%d/%d/%s", s.Type, s.Clauses, s.Path)
		assert.Contains(t, want, k, "unexpected spec: %s", k)
	}

	// No 0x02 multi-clause.
	for _, s := range m {
		if s.Type == 0x02 {
			assert.Equal(t, 1, s.Clauses, "0x02 must be single-clause")
		}
	}
}

func TestBuild_0x00_SingleClause(t *testing.T) {
	devAccs := genesis.DevAccounts()
	key := devAccs[0].PrivateKey
	spec := Spec{Type: 0x00, Clauses: 1, Path: "rest"}

	trx, err := Build(spec, key, 0, testChainTag, testChainID, testBlockRef)
	require.NoError(t, err)
	assert.Equal(t, tx.TypeLegacy, trx.Type())
	assert.Len(t, trx.Clauses(), 1)
	origin, err := trx.Origin()
	require.NoError(t, err)
	assert.Equal(t, devAccs[0].Address, origin)
}

func TestBuild_0x00_MultiClause(t *testing.T) {
	devAccs := genesis.DevAccounts()
	key := devAccs[0].PrivateKey
	spec := Spec{Type: 0x00, Clauses: 3, Path: "rest"}

	trx, err := Build(spec, key, 0, testChainTag, testChainID, testBlockRef)
	require.NoError(t, err)
	assert.Equal(t, tx.TypeLegacy, trx.Type())
	assert.Len(t, trx.Clauses(), 3)
	origin, err := trx.Origin()
	require.NoError(t, err)
	assert.Equal(t, devAccs[0].Address, origin)
}

func TestBuild_0x51_SingleClause(t *testing.T) {
	devAccs := genesis.DevAccounts()
	key := devAccs[0].PrivateKey
	spec := Spec{Type: 0x51, Clauses: 1, Path: "rest"}

	trx, err := Build(spec, key, 0, testChainTag, testChainID, testBlockRef)
	require.NoError(t, err)
	assert.Equal(t, tx.TypeDynamicFee, trx.Type())
	assert.Len(t, trx.Clauses(), 1)
	origin, err := trx.Origin()
	require.NoError(t, err)
	assert.Equal(t, devAccs[0].Address, origin)
}

func TestBuild_0x51_MultiClause(t *testing.T) {
	devAccs := genesis.DevAccounts()
	key := devAccs[0].PrivateKey
	spec := Spec{Type: 0x51, Clauses: 3, Path: "rest"}

	trx, err := Build(spec, key, 0, testChainTag, testChainID, testBlockRef)
	require.NoError(t, err)
	assert.Equal(t, tx.TypeDynamicFee, trx.Type())
	assert.Len(t, trx.Clauses(), 3)
	origin, err := trx.Origin()
	require.NoError(t, err)
	assert.Equal(t, devAccs[0].Address, origin)
}

func TestBuild_0x02_SingleClause(t *testing.T) {
	devAccs := genesis.DevAccounts()
	key := devAccs[0].PrivateKey
	spec := Spec{Type: 0x02, Clauses: 1, Path: "rest"}

	trx, err := Build(spec, key, 42, testChainTag, testChainID, testBlockRef)
	require.NoError(t, err)
	assert.Equal(t, tx.TypeEthDynamicFee, trx.Type())
	assert.Len(t, trx.Clauses(), 1)
	origin, err := trx.Origin()
	require.NoError(t, err)
	assert.Equal(t, devAccs[0].Address, origin)
}

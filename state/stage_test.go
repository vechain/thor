// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/lvldb"
	"github.com/vechain/thor/thor"
)

func TestStage(t *testing.T) {
	kv, _ := lvldb.NewMem()
	state, _ := New(thor.Bytes32{}, kv)

	addr := thor.BytesToAddress([]byte("acc1"))

	balance := big.NewInt(10)
	code := []byte{1, 2, 3}

	storage := map[thor.Bytes32]thor.Bytes32{
		thor.BytesToBytes32([]byte("s1")): thor.BytesToBytes32([]byte("v1")),
		thor.BytesToBytes32([]byte("s2")): thor.BytesToBytes32([]byte("v2")),
		thor.BytesToBytes32([]byte("s3")): thor.BytesToBytes32([]byte("v3"))}

	state.SetBalance(addr, balance)
	state.SetCode(addr, code)
	for k, v := range storage {
		state.SetStorage(addr, k, v)
	}

	stage := state.Stage()

	hash, err := stage.Hash()
	assert.Nil(t, err)
	root, err := stage.Commit()
	assert.Nil(t, err)

	assert.Equal(t, hash, root)

	state, _ = New(root, kv)

	assert.Equal(t, balance, state.GetBalance(addr))
	assert.Equal(t, code, state.GetCode(addr))
	assert.Equal(t, thor.Bytes32(crypto.Keccak256Hash(code)), state.GetCodeHash(addr))
	for k, v := range storage {
		assert.Equal(t, v, state.GetStorage(addr, k))
	}
}

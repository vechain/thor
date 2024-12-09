// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func TestStage(t *testing.T) {
	db := muxdb.NewMem()
	state := New(db, trie.Root{})
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

	stage, err := state.Stage(trie.Version{Major: 1})
	assert.Nil(t, err)

	hash := stage.Hash()

	root, err := stage.Commit()
	assert.Nil(t, err)

	assert.Equal(t, hash, root)

	state = New(db, trie.Root{Hash: root, Ver: trie.Version{Major: 1}})

	assert.Equal(t, M(balance, nil), M(state.GetBalance(addr)))
	assert.Equal(t, M(code, nil), M(state.GetCode(addr)))
	assert.Equal(t, M(thor.Keccak256(code), nil), M(state.GetCodeHash(addr)))

	for k, v := range storage {
		assert.Equal(t, M(v, nil), M(state.GetStorage(addr, k)))
	}
}

func TestStageCommitError(t *testing.T) {
	state := New(muxdb.NewMem(), trie.Root{})

	// Set up the state with an account, balance, code, and storage.
	addr := thor.BytesToAddress([]byte("acc1"))
	balance := big.NewInt(10)
	code := []byte{1, 2, 3}
	storage := map[thor.Bytes32]thor.Bytes32{
		thor.BytesToBytes32([]byte("s1")): thor.BytesToBytes32([]byte("v1")),
		thor.BytesToBytes32([]byte("s2")): thor.BytesToBytes32([]byte("v2")),
		thor.BytesToBytes32([]byte("s3")): thor.BytesToBytes32([]byte("v3")),
	}

	state.SetBalance(addr, balance)
	state.SetCode(addr, code)
	for k, v := range storage {
		state.SetStorage(addr, k, v)
	}

	// Prepare the stage with the current state.
	stage, err := state.Stage(trie.Version{Major: 1})
	assert.Nil(t, err, "Stage should not return an error")

	// Mock a commit function to simulate an error.
	commitFuncWithError := func() error {
		return errors.New("commit error")
	}

	// Include the error-producing commit function in the stage's commits.
	stage.commit = commitFuncWithError

	// Attempt to commit changes.
	_, err = stage.Commit()

	// Assert that an error is returned.
	assert.NotNil(t, err, "Commit should return an error")
	assert.EqualError(t, err, "state: commit error", "The error message should match the mock error")
}

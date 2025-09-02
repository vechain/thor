// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package validation

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/builtin/solidity"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func newRepo(t *testing.T) (*Repository, thor.Address, *state.State) {
	db := muxdb.NewMem()
	st := state.New(db, trie.Root{})
	addr := thor.BytesToAddress([]byte("valrepo"))
	repo := NewRepository(solidity.NewContext(addr, st, nil))
	return repo, addr, st
}

func TestRepository_Validation_RoundTrip(t *testing.T) {
	repo, _, _ := newRepo(t)
	id := thor.BytesToAddress([]byte("v1"))
	entry := &Validation{
		Endorser:           thor.BytesToAddress([]byte("e1")),
		Period:             15,
		CompleteIterations: 2,
		Status:             StatusQueued,
	}

	assert.NoError(t, repo.addValidation(id, entry))

	got, err := repo.getValidation(id)
	assert.NoError(t, err)
	assert.Equal(t, entry.Endorser, got.Endorser)
	assert.Equal(t, uint32(15), got.Period)
	assert.Equal(t, uint32(2), got.CompleteIterations)
	assert.Equal(t, StatusQueued, got.Status)
	assert.True(t, got.OfflineBlock == nil)
}

func TestRepository_Validation_GetError(t *testing.T) {
	repo, addr, st := newRepo(t)
	id := thor.BytesToAddress([]byte("v2"))

	// Poison the validations mapping slot so Mapping.Get fails
	slot := thor.Blake2b(id.Bytes(), slotValidations.Bytes())
	st.SetRawStorage(addr, slot, rlp.RawValue{0xFF})

	_, err := repo.getValidation(id)
	assert.ErrorContains(t, err, "failed to get validator")
}

func TestRepository_Reward_RoundTrip_DefaultZero(t *testing.T) {
	repo, _, _ := newRepo(t)
	key := thor.BytesToBytes32([]byte("r1"))

	// get before set -> zero
	val, err := repo.getReward(key)
	assert.NoError(t, err)
	assert.Equal(t, big.NewInt(0), val)

	// set then get
	want := big.NewInt(1234)
	assert.NoError(t, repo.setReward(key, want, true))

	got, err := repo.getReward(key)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestRepository_Reward_GetError(t *testing.T) {
	repo, addr, st := newRepo(t)
	key := thor.BytesToBytes32([]byte("r2"))

	// Poison rewards slot for key
	slot := thor.Blake2b(key.Bytes(), slotRewards.Bytes())
	st.SetRawStorage(addr, slot, rlp.RawValue{0xFF})

	_, err := repo.getReward(key)
	assert.ErrorContains(t, err, "failed to get reward")
}

func TestRepository_Exit_RoundTrip(t *testing.T) {
	repo, _, _ := newRepo(t)
	validator := thor.BytesToAddress([]byte("v3"))

	assert.NoError(t, repo.setExit(42, validator))

	addr, err := repo.getExit(42)
	assert.NoError(t, err)
	assert.Equal(t, validator, addr)
}

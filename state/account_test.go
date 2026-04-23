// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

func M(a ...any) []any {
	return a
}

func TestAccount(t *testing.T) {
	assert.True(t, emptyAccount().IsEmpty())

	acc := emptyAccount()
	acc.Balance = big.NewInt(1)
	assert.False(t, acc.IsEmpty())
	acc = emptyAccount()
	acc.CodeHash = []byte{1}
	assert.False(t, acc.IsEmpty())

	acc = emptyAccount()
	acc.Energy = big.NewInt(1)
	assert.False(t, acc.IsEmpty())

	acc = emptyAccount()
	acc.StorageRoot = []byte{1}
	assert.True(t, acc.IsEmpty())

	acc = emptyAccount()
	acc.Nonce = 1
	assert.False(t, acc.IsEmpty())
}

// TestAccountRLPBackwardCompat guards the `rlp:"optional"` contract: existing
// on-chain account blobs (6-field RLP) must still decode, and a new-format
// account with Nonce==0 must encode back to the same 6-field blob so state
// root is unchanged for non-ETH-tx writers.
func TestAccountRLPBackwardCompat(t *testing.T) {
	// Legacy 6-field encoding (as stored pre-INTERSTELLAR).
	type legacyAccount struct {
		Balance     *big.Int
		Energy      *big.Int
		BlockTime   uint64
		Master      []byte
		CodeHash    []byte
		StorageRoot []byte
	}
	legacy := legacyAccount{
		Balance:     big.NewInt(7),
		Energy:      big.NewInt(11),
		BlockTime:   13,
		Master:      []byte("master"),
		CodeHash:    []byte("codehash"),
		StorageRoot: []byte("sroot"),
	}
	legacyBytes, err := rlp.EncodeToBytes(&legacy)
	assert.NoError(t, err)

	var decoded Account
	assert.NoError(t, rlp.DecodeBytes(legacyBytes, &decoded))
	assert.Equal(t, uint64(0), decoded.Nonce, "old encoding must decode to Nonce=0")
	assert.Equal(t, legacy.Balance, decoded.Balance)

	// Encoding a new-format account with Nonce==0 must produce byte-identical
	// output to the legacy 6-field encoding (optional tag strips trailing zero).
	newZero := Account{
		Balance:     big.NewInt(7),
		Energy:      big.NewInt(11),
		BlockTime:   13,
		Master:      []byte("master"),
		CodeHash:    []byte("codehash"),
		StorageRoot: []byte("sroot"),
		Nonce:       0,
	}
	newZeroBytes, err := rlp.EncodeToBytes(&newZero)
	assert.NoError(t, err)
	assert.Equal(t, legacyBytes, newZeroBytes, "Nonce==0 must encode as 6-field (same as legacy)")

	// Non-zero Nonce must encode as 7 fields and round-trip cleanly.
	newNonce := newZero
	newNonce.Nonce = 42
	newNonceBytes, err := rlp.EncodeToBytes(&newNonce)
	assert.NoError(t, err)
	assert.NotEqual(t, legacyBytes, newNonceBytes, "Nonce>0 must produce different encoding")

	var rt Account
	assert.NoError(t, rlp.DecodeBytes(newNonceBytes, &rt))
	assert.Equal(t, uint64(42), rt.Nonce)
}

func TestCalculateEnergy(t *testing.T) {
	assert.True(t, emptyAccount().IsEmpty())

	acc := emptyAccount()
	acc.Energy = big.NewInt(1)
	energy := acc.CalcEnergy(1, 10)
	assert.Equal(t, big.NewInt(1), energy)

	acc.BlockTime = 1
	energy = acc.CalcEnergy(1, 10)
	assert.Equal(t, big.NewInt(1), energy)

	acc.Balance = big.NewInt(100000000000000000)
	energy = acc.CalcEnergy(1, 10)
	assert.Equal(t, big.NewInt(1), energy)

	energy = acc.CalcEnergy(10, 10)
	assert.Equal(t, big.NewInt(4500000001), energy)

	energy = acc.CalcEnergy(20, 10)
	assert.Equal(t, big.NewInt(4500000001), energy)

	acc.BlockTime = 10
	energy = acc.CalcEnergy(20, 10)
	assert.Equal(t, big.NewInt(1), energy)
}

func TestTrie(t *testing.T) {
	db := muxdb.NewMem()
	tr := db.NewTrie("", trie.Root{})

	addr := thor.BytesToAddress([]byte("account1"))
	assert.Equal(t,
		M(loadAccount(tr, addr)),
		M(emptyAccount(), &AccountMetadata{}, nil),
		"should load an empty account")

	acc1 := Account{
		Balance:     big.NewInt(1),
		Energy:      big.NewInt(0),
		BlockTime:   0,
		Master:      []byte("master"),
		CodeHash:    []byte("code hash"),
		StorageRoot: []byte("storage root"),
	}
	meta1 := AccountMetadata{
		StorageID:       []byte("sid"),
		StorageMajorVer: 1,
		StorageMinorVer: 2,
	}
	saveAccount(tr, addr, &acc1, &meta1)
	assert.Equal(t,
		M(loadAccount(tr, addr)),
		M(&acc1, &meta1, nil))

	saveAccount(tr, addr, emptyAccount(), &meta1)
	assert.Equal(t,
		M(tr.Get(addr[:])),
		M([]byte(nil), []byte(nil), nil),
		"empty account should be deleted")
}

func TestStorageTrie(t *testing.T) {
	db := muxdb.NewMem()
	tr := db.NewTrie("", trie.Root{})

	key := thor.BytesToBytes32([]byte("key"))
	assert.Equal(t,
		M(loadStorage(tr, key)),
		M(rlp.RawValue(nil), nil))

	value := rlp.RawValue("value")
	saveStorage(tr, key, value)
	assert.Equal(t,
		M(loadStorage(tr, key)),
		M(value, nil))

	saveStorage(tr, key, nil)
	assert.Equal(t,
		M(tr.Get(key[:])),
		M([]byte(nil), []byte(nil), nil),
		"empty storage value should be deleted")
}

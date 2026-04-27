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

type OldAccount struct {
	Balance     *big.Int
	Energy      *big.Int
	BlockTime   uint64
	Master      []byte
	CodeHash    []byte
	StorageRoot []byte
}

func M(a ...any) []any {
	return a
}

func TestAccountRLPOptional(t *testing.T) {
	// Encode an Account (without Nonce) and decode into NewAccount
	oldAcc := OldAccount{
		Balance:     big.NewInt(100),
		Energy:      big.NewInt(200),
		BlockTime:   1000,
		Master:      []byte("master"),
		CodeHash:    []byte("codehash"),
		StorageRoot: []byte("storageroot"),
	}

	encoded, err := rlp.EncodeToBytes(&oldAcc)
	assert.NoError(t, err)

	var newAcc Account
	err = rlp.DecodeBytes(encoded, &newAcc)
	assert.NoError(t, err, "decoding OldAccount bytes into Account should succeed with optional field")

	// The original fields should match
	assert.Equal(t, oldAcc.Balance.Int64(), newAcc.Balance.Int64())
	assert.Equal(t, oldAcc.Energy.Int64(), newAcc.Energy.Int64())
	assert.Equal(t, oldAcc.BlockTime, newAcc.BlockTime)
	assert.Equal(t, oldAcc.Master, newAcc.Master)
	assert.Equal(t, oldAcc.CodeHash, newAcc.CodeHash)
	assert.Equal(t, oldAcc.StorageRoot, newAcc.StorageRoot)

	// The optional Nonce should default to zero
	assert.Equal(t, uint64(0), newAcc.Nonce)

	// Encode a NewAccount with Nonce set, decode back into NewAccount
	newAcc2 := Account{
		Balance:     big.NewInt(100),
		Energy:      big.NewInt(200),
		BlockTime:   1000,
		Master:      []byte("master"),
		CodeHash:    []byte("codehash"),
		StorageRoot: []byte("storageroot"),
		Nonce:       42,
	}

	encoded2, err := rlp.EncodeToBytes(&newAcc2)
	assert.NoError(t, err)

	var newAcc3 Account
	err = rlp.DecodeBytes(encoded2, &newAcc3)
	assert.NoError(t, err)
	assert.Equal(t, uint64(42), newAcc3.Nonce)

	// Encode a NewAccount with Nonce set, decode into old Account
	// RLP does NOT allow extra fields — old struct rejects new data with additional fields
	var oldAcc2 OldAccount
	err = rlp.DecodeBytes(encoded2, &oldAcc2)
	assert.Error(t, err, "decoding NewAccount bytes (with extra field) into OldAccount should fail")

	// When Nonce=0 (zero value), rlp:"optional" omits it from encoding,
	// so OldAccount CAN decode it — backward compatible when optional field is zero.
	newAcc5 := Account{
		Balance:   big.NewInt(100),
		Energy:    big.NewInt(200),
		BlockTime: 1000,
		Nonce:     0,
	}
	encoded5, err := rlp.EncodeToBytes(&newAcc5)
	assert.NoError(t, err)

	var oldAcc3 OldAccount
	err = rlp.DecodeBytes(encoded5, &oldAcc3)
	assert.NoError(t, err, "Account with zero optional field is backward compatible with OldAccount")
	assert.Equal(t, newAcc5.Balance.Int64(), oldAcc3.Balance.Int64())
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

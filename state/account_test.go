// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ethereum/go-ethereum/rlp"

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
		big.NewInt(1),
		big.NewInt(0),
		0,
		[]byte("master"),
		[]byte("code hash"),
		[]byte("storage root"),
		0,
	}
	meta1 := AccountMetadata{
		StorageID:       []byte("sid"),
		StorageMajorVer: 1,
		StorageMinorVer: 2,
	}
	if err := saveAccount(tr, addr, &acc1, &meta1); err != nil {
		t.Fatalf("saveAccount: %v", err)
	}
	assert.Equal(t,
		M(loadAccount(tr, addr)),
		M(&acc1, &meta1, nil))

	saveAccount(tr, addr, emptyAccount(), &meta1)
	assert.Equal(t,
		M(tr.Get(addr[:])),
		M([]byte(nil), []byte(nil), nil),
		"empty account should be deleted")
}

// TestAccountRLPBackCompat pins the wire format:
//   - Nonce==0 encodes as the legacy 6-field layout (state root unchanged for
//     all historic accounts) via rlp:"optional".
//   - Nonce>0 encodes as the 7-field layout.
//   - Both decode back into a structurally equivalent Account.
func TestAccountRLPBackCompat(t *testing.T) {
	common := Account{
		Balance:     big.NewInt(1),
		Energy:      big.NewInt(2),
		BlockTime:   3,
		Master:      []byte("master"),
		CodeHash:    []byte("code hash"),
		StorageRoot: []byte("storage root"),
	}

	// fieldCount returns the number of values in the encoded RLP list.
	fieldCount := func(b []byte) int {
		t.Helper()
		content, _, err := rlp.SplitList(b)
		assert.Nil(t, err)
		n, err := rlp.CountValues(content)
		assert.Nil(t, err)
		return n
	}

	// legacy 6-field reference encoding.
	type legacy struct {
		Balance     *big.Int
		Energy      *big.Int
		BlockTime   uint64
		Master      []byte
		CodeHash    []byte
		StorageRoot []byte
	}
	legacyBytes, err := rlp.EncodeToBytes(&legacy{
		Balance: common.Balance, Energy: common.Energy, BlockTime: common.BlockTime,
		Master: common.Master, CodeHash: common.CodeHash, StorageRoot: common.StorageRoot,
	})
	assert.Nil(t, err)
	assert.Equal(t, 6, fieldCount(legacyBytes))

	// Nonce==0 → 6-field encoding, byte-for-byte equal to legacy reference.
	zeroAcc := common
	zeroBytes, err := rlp.EncodeToBytes(&zeroAcc)
	assert.Nil(t, err)
	assert.Equal(t, 6, fieldCount(zeroBytes), "Nonce==0 must encode as 6 fields")
	assert.Equal(t, legacyBytes, zeroBytes, "Nonce==0 must encode bit-for-bit as legacy")

	// Nonce>0 → 7-field encoding.
	noncedAcc := common
	noncedAcc.Nonce = 42
	noncedBytes, err := rlp.EncodeToBytes(&noncedAcc)
	assert.Nil(t, err)
	assert.Equal(t, 7, fieldCount(noncedBytes), "Nonce>0 must encode as 7 fields")
	assert.NotEqual(t, legacyBytes, noncedBytes)

	// Decode 6-field bytes → Account with Nonce=0.
	var decodedZero Account
	assert.Nil(t, rlp.DecodeBytes(legacyBytes, &decodedZero))
	assert.Equal(t, uint64(0), decodedZero.Nonce)

	// Decode 7-field bytes → Account with Nonce=42.
	var decodedNonced Account
	assert.Nil(t, rlp.DecodeBytes(noncedBytes, &decodedNonced))
	assert.Equal(t, uint64(42), decodedNonced.Nonce)
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

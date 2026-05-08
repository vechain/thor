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

type NoNonceAccount struct {
	Balance     *big.Int
	Energy      *big.Int
	BlockTime   uint64
	Master      []byte // master address
	CodeHash    []byte // hash of code
	StorageRoot []byte // merkle root of the storage trie
}

func TestNoNonceAccountRLPCompatibility(t *testing.T) {
	legacy := NoNonceAccount{
		Balance:     big.NewInt(100),
		Energy:      big.NewInt(200),
		BlockTime:   123456,
		Master:      []byte("master"),
		CodeHash:    []byte("code hash"),
		StorageRoot: []byte("storage root"),
	}

	data, err := rlp.EncodeToBytes(&legacy)
	assert.Nil(t, err)

	var decoded Account
	err = rlp.DecodeBytes(data, &decoded)
	assert.Nil(t, err)

	assert.Equal(t, legacy.Balance, decoded.Balance)
	assert.Equal(t, legacy.Energy, decoded.Energy)
	assert.Equal(t, legacy.BlockTime, decoded.BlockTime)
	assert.Equal(t, legacy.Master, decoded.Master)
	assert.Equal(t, legacy.CodeHash, decoded.CodeHash)
	assert.Equal(t, legacy.StorageRoot, decoded.StorageRoot)
	assert.Equal(t, uint64(0), decoded.Nonce, "Nonce should default to 0 when missing from RLP")
}

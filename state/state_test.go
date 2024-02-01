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

func TestStateReadWrite(t *testing.T) {
	db := muxdb.NewMem()

	state := New(db, trie.Root{})

	addr := thor.BytesToAddress([]byte("account1"))
	storageKey := thor.BytesToBytes32([]byte("storageKey"))

	assert.Equal(t, M(false, nil), M(state.Exists(addr)))
	assert.Equal(t, M(&big.Int{}, nil), M(state.GetBalance(addr)))
	assert.Equal(t, M([]byte(nil), nil), M(state.GetCode(addr)))
	assert.Equal(t, M(thor.Bytes32{}, nil), M(state.GetCodeHash(addr)))
	assert.Equal(t, M(thor.Bytes32{}, nil), M(state.GetStorage(addr, storageKey)))

	// make account not empty
	state.SetBalance(addr, big.NewInt(1))
	assert.Equal(t, M(big.NewInt(1), nil), M(state.GetBalance(addr)))

	state.SetMaster(addr, thor.BytesToAddress([]byte("master")))
	assert.Equal(t, M(thor.BytesToAddress([]byte("master")), nil), M(state.GetMaster(addr)))

	state.SetCode(addr, []byte("code"))
	assert.Equal(t, M([]byte("code"), nil), M(state.GetCode(addr)))
	assert.Equal(t, M(thor.Keccak256([]byte("code")), nil), M(state.GetCodeHash(addr)))

	assert.Equal(t, M(thor.Bytes32{}, nil), M(state.GetStorage(addr, storageKey)))
	state.SetStorage(addr, storageKey, thor.BytesToBytes32([]byte("storageValue")))
	assert.Equal(t, M(thor.BytesToBytes32([]byte("storageValue")), nil), M(state.GetStorage(addr, storageKey)))

	assert.Equal(t, M(true, nil), M(state.Exists(addr)))

	// delete account
	state.Delete(addr)
	assert.Equal(t, M(false, nil), M(state.Exists(addr)))
	assert.Equal(t, M(&big.Int{}, nil), M(state.GetBalance(addr)))
	assert.Equal(t, M(thor.Address{}, nil), M(state.GetMaster(addr)))
	assert.Equal(t, M([]byte(nil), nil), M(state.GetCode(addr)))
	assert.Equal(t, M(thor.Bytes32{}, nil), M(state.GetCodeHash(addr)))
}

func TestStateRevert(t *testing.T) {
	db := muxdb.NewMem()
	state := New(db, trie.Root{})

	addr := thor.BytesToAddress([]byte("account1"))
	storageKey := thor.BytesToBytes32([]byte("storageKey"))

	values := []struct {
		balance *big.Int
		code    []byte
		storage thor.Bytes32
	}{
		{big.NewInt(1), []byte("code1"), thor.BytesToBytes32([]byte("v1"))},
		{big.NewInt(2), []byte("code2"), thor.BytesToBytes32([]byte("v2"))},
		{big.NewInt(3), []byte("code3"), thor.BytesToBytes32([]byte("v3"))},
	}

	var chk int
	for _, v := range values {
		chk = state.NewCheckpoint()
		state.SetBalance(addr, v.balance)
		state.SetCode(addr, v.code)
		state.SetStorage(addr, storageKey, v.storage)
	}

	for i := range values {
		v := values[len(values)-i-1]
		assert.Equal(t, M(v.balance, nil), M(state.GetBalance(addr)))
		assert.Equal(t, M(v.code, nil), M(state.GetCode(addr)))
		assert.Equal(t, M(thor.Keccak256(v.code), nil), M(state.GetCodeHash(addr)))
		assert.Equal(t, M(v.storage, nil), M(state.GetStorage(addr, storageKey)))
		state.RevertTo(chk)
		chk--
	}
	assert.Equal(t, M(false, nil), M(state.Exists(addr)))

	//
	state = New(db, trie.Root{})
	assert.Equal(t, state.NewCheckpoint(), 1)
	state.RevertTo(0)
	assert.Equal(t, state.NewCheckpoint(), 0)
}

func TestEnergy(t *testing.T) {
	db := muxdb.NewMem()
	st := New(db, trie.Root{})

	acc := thor.BytesToAddress([]byte("a1"))

	time1 := uint64(1000)

	vetBal := big.NewInt(1e18)
	st.SetBalance(acc, vetBal)
	st.SetEnergy(acc, &big.Int{}, 10)

	bal1, _ := st.GetEnergy(acc, time1)
	x := new(big.Int).Mul(thor.EnergyGrowthRate, vetBal)
	x.Mul(x, new(big.Int).SetUint64(time1-10))
	x.Div(x, big.NewInt(1e18))

	assert.Equal(t, x, bal1)
}

func TestEncodeDecodeStorage(t *testing.T) {
	db := muxdb.NewMem()
	state := New(db, trie.Root{})

	// Create an account and key
	addr := thor.BytesToAddress([]byte("account1"))
	key := thor.BytesToBytes32([]byte("key"))

	// Original value to be encoded and then decoded
	originalValue := []byte("value")

	// Encoding function
	encodingFunc := func() ([]byte, error) {
		return rlp.EncodeToBytes(originalValue)
	}

	// Encode and store the value
	err := state.EncodeStorage(addr, key, encodingFunc)
	assert.Nil(t, err, "EncodeStorage should not return an error")

	// Function to decode the storage value
	var decodedValue []byte
	decodeFunc := func(b []byte) error {
		return rlp.DecodeBytes(b, &decodedValue)
	}

	// Decode the stored value
	err = state.DecodeStorage(addr, key, decodeFunc)
	assert.Nil(t, err, "DecodeStorage should not return an error")

	// Verify that the decoded value matches the original value
	assert.Equal(t, originalValue, decodedValue, "decoded value should match the original value")
}

func TestBuildStorageTrie(t *testing.T) {
	db := muxdb.NewMem()
	state := New(db, trie.Root{})

	// Create an account and set storage values
	addr := thor.BytesToAddress([]byte("account1"))
	key1 := thor.BytesToBytes32([]byte("key1"))
	value1 := thor.BytesToBytes32([]byte("value1"))
	key2 := thor.BytesToBytes32([]byte("key2"))
	value2 := thor.BytesToBytes32([]byte("value2"))

	state.SetRawStorage(addr, key1, value1[:])
	state.SetRawStorage(addr, key2, value2[:])

	assert.Equal(t, M(rlp.RawValue(value1[:]), nil), M(state.GetRawStorage(addr, key1)))

	// Build the storage trie
	_, err := state.BuildStorageTrie(addr)
	assert.Nil(t, err, "error should be nil")
}

func TestStorage(t *testing.T) {
	db := muxdb.NewMem()
	st := New(db, trie.Root{})

	addr := thor.BytesToAddress([]byte("addr"))
	key := thor.BytesToBytes32([]byte("key"))

	st.SetStorage(addr, key, thor.BytesToBytes32([]byte{1}))
	data, _ := rlp.EncodeToBytes([]byte{1})
	assert.Equal(t, M(rlp.RawValue(data), nil), M(st.GetRawStorage(addr, key)))

	st.SetRawStorage(addr, key, data)
	assert.Equal(t, M(thor.BytesToBytes32([]byte{1}), nil), M(st.GetStorage(addr, key)))

	st.SetStorage(addr, key, thor.Bytes32{})
	assert.Equal(t, M(rlp.RawValue(nil), nil), M(st.GetRawStorage(addr, key)))

	v := struct {
		V1 uint
	}{313123}

	data, _ = rlp.EncodeToBytes(&v)
	st.SetRawStorage(addr, key, data)

	assert.Equal(t, M(thor.Blake2b(data), nil), M(st.GetStorage(addr, key)))
}

func TestStorageBarrier(t *testing.T) {
	db := muxdb.NewMem()
	st := New(db, trie.Root{})

	addr := thor.BytesToAddress([]byte("addr"))
	key := thor.BytesToBytes32([]byte("key"))

	st.SetCode(addr, []byte("code"))
	st.SetStorage(addr, key, thor.BytesToBytes32([]byte("data")))

	st.Delete(addr)
	assert.Equal(t, M(rlp.RawValue(nil), nil), M(st.GetRawStorage(addr, key)), "should read empty storage when account deleted")

	st.SetCode(addr, []byte("code"))

	stage, err := st.Stage(trie.Version{})
	assert.Nil(t, err)

	root, err := stage.Commit()
	assert.Nil(t, err)

	tr := db.NewTrie(AccountTrieName, trie.Root{Hash: root})
	acc, _, err := loadAccount(tr, addr)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(acc.StorageRoot), "should skip storage writes when account deleteed then recreated")
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"encoding/binary"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
)

// AccountMetadata helps encode/decode account metadata.
type AccountMetadata []byte

// NewAccountMetadata builds the account metadata. addr is optional.
func NewAccountMetadata(storageCommitNum, storageInitCommitNum uint32, addr *thor.Address) AccountMetadata {
	var buf []byte
	if addr != nil {
		buf = make([]byte, 28)
		copy(buf[8:], addr[:])
	} else {
		buf = make([]byte, 8)
	}
	binary.BigEndian.PutUint32(buf, storageCommitNum)
	binary.BigEndian.PutUint32(buf[4:], storageInitCommitNum)
	return buf
}

// StorageCommitNum returns the storage commit number.
func (m AccountMetadata) StorageCommitNum() uint32 {
	return binary.BigEndian.Uint32(m)
}

// StorageInitCommitNum returns the initial storage commit number.
func (m AccountMetadata) StorageInitCommitNum() uint32 {
	return binary.BigEndian.Uint32(m[4:])
}

// Address returns the account address.
func (m AccountMetadata) Address() (thor.Address, bool) {
	if len(m) == 8 {
		return thor.Address{}, false
	}
	return thor.BytesToAddress(m[8:28]), true
}

// SkipAddress returns the account metadata without address.
func (m AccountMetadata) SkipAddress() AccountMetadata {
	return m[:8]
}

// Account is the Thor consensus representation of an account.
// RLP encoded objects are stored in main account trie.
type Account struct {
	Balance     *big.Int
	Energy      *big.Int
	BlockTime   uint64
	Master      []byte // master address
	CodeHash    []byte // hash of code
	StorageRoot []byte // merkle root of the storage trie

	storageCommitNum, storageInitCommitNum uint32
}

// IsEmpty returns if an account is empty.
// An empty account has zero balance and zero length code hash.
func (a *Account) IsEmpty() bool {
	return a.Balance.Sign() == 0 &&
		a.Energy.Sign() == 0 &&
		len(a.Master) == 0 &&
		len(a.CodeHash) == 0
}

var bigE18 = big.NewInt(1e18)

// CalcEnergy calculates energy based on current block time.
func (a *Account) CalcEnergy(blockTime uint64) *big.Int {
	if a.BlockTime == 0 {
		return a.Energy
	}

	if a.Balance.Sign() == 0 {
		return a.Energy
	}

	if blockTime <= a.BlockTime {
		return a.Energy
	}

	x := new(big.Int).SetUint64(blockTime - a.BlockTime)
	x.Mul(x, a.Balance)
	x.Mul(x, thor.EnergyGrowthRate)
	x.Div(x, bigE18)
	return new(big.Int).Add(a.Energy, x)
}

func emptyAccount() *Account {
	a := Account{Balance: &big.Int{}, Energy: &big.Int{}}
	return &a
}

// loadAccount load an account object by address in trie.
// It returns empty account is no account found at the address.
func loadAccount(trie *muxdb.Trie, addr thor.Address, steadyCommitNum uint32) (*Account, error) {
	data, meta, err := trie.FastGet(addr[:], steadyCommitNum)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return emptyAccount(), nil
	}
	var a Account
	if err := rlp.DecodeBytes(data, &a); err != nil {
		return nil, err
	}
	am := AccountMetadata(meta)
	a.storageCommitNum, a.storageInitCommitNum = am.StorageCommitNum(), am.StorageInitCommitNum()
	return &a, nil
}

// saveAccount save account into trie at given address.
// If the given account is empty, the value for given address is deleted.
func saveAccount(trie *muxdb.Trie, addr thor.Address, a *Account) error {
	if a.IsEmpty() {
		// delete if account is empty
		return trie.Update(addr[:], nil, nil)
	}

	data, err := rlp.EncodeToBytes(a)
	if err != nil {
		return err
	}

	am := NewAccountMetadata(a.storageCommitNum, a.storageInitCommitNum, &addr)
	return trie.Update(addr[:], data, am)
}

// loadStorage load storage data for given key.
func loadStorage(trie *muxdb.Trie, key thor.Bytes32, steadyCommitNum uint32) (rlp.RawValue, error) {
	v, _, err := trie.FastGet(key[:], steadyCommitNum)
	return v, err
}

// saveStorage save value for given key.
// If the data is zero, the given key will be deleted.
func saveStorage(trie *muxdb.Trie, key thor.Bytes32, data rlp.RawValue) error {
	return trie.Update(
		key[:],
		data,
		key[:], // key preimage as metadata
	)
}

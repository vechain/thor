// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
)

// AccountMetadata is the account metadata.
type AccountMetadata struct {
	StorageID          []byte // the unique id of the storage trie.
	StorageCommitNum   uint32 // the commit number of the last storage update.
	StorageDistinctNum uint32 // the distinct number of the last storage update.
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

// loadAccount load an account object and its metadata by address in trie.
// It returns empty account is no account found at the address.
func loadAccount(trie *muxdb.Trie, addr thor.Address, steadyBlockNum uint32) (*Account, *AccountMetadata, error) {
	hashedKey := thor.Blake2b(addr[:])
	data, meta, err := trie.FastGet(hashedKey[:], steadyBlockNum)
	if err != nil {
		return nil, nil, err
	}
	if len(data) == 0 {
		return emptyAccount(), &AccountMetadata{}, nil
	}
	var a Account
	if err := rlp.DecodeBytes(data, &a); err != nil {
		return nil, nil, err
	}

	var am AccountMetadata
	if len(meta) > 0 {
		if err := rlp.DecodeBytes(meta, &am); err != nil {
			return nil, nil, err
		}
	}
	return &a, &am, nil
}

// saveAccount save account into trie at given address.
// If the given account is empty, the value for given address is deleted.
func saveAccount(trie *muxdb.Trie, addr thor.Address, a *Account, am *AccountMetadata) error {
	if a.IsEmpty() {
		hashedKey := thor.Blake2b(addr[:])
		// delete if account is empty
		return trie.Update(hashedKey[:], nil, nil)
	}

	data, err := rlp.EncodeToBytes(a)
	if err != nil {
		return err
	}

	var mdata []byte
	if len(a.StorageRoot) > 0 { // discard metadata if storage root is empty
		if mdata, err = rlp.EncodeToBytes(am); err != nil {
			return err
		}
	}
	hashedKey := thor.Blake2b(addr[:])
	return trie.Update(hashedKey[:], data, mdata)
}

// loadStorage load storage data for given key.
func loadStorage(trie *muxdb.Trie, key thor.Bytes32, steadyBlockNum uint32) (rlp.RawValue, error) {
	hashedKey := thor.Blake2b(key[:])
	v, _, err := trie.FastGet(
		hashedKey[:],
		steadyBlockNum)
	return v, err
}

// saveStorage save value for given key.
// If the data is zero, the given key will be deleted.
func saveStorage(trie *muxdb.Trie, key thor.Bytes32, data rlp.RawValue) error {
	hashedKey := thor.Blake2b(key[:])
	return trie.Update(
		hashedKey[:],
		data,
		bytes.TrimLeft(key[:], "\x00"), // key preimage as metadata
	)
}

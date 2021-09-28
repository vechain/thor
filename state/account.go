// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
)

// AccountMetadata includes account metadata.
type AccountMetadata struct {
	Addr                 thor.Address
	StorageCommitNum     uint32
	StorageInitCommitNum uint32
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

	meta AccountMetadata
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

func emptyAccount(addr thor.Address) *Account {
	a := Account{Balance: &big.Int{}, Energy: &big.Int{}}
	a.meta.Addr = addr
	return &a
}

// loadAccount load an account object by address in trie.
// It returns empty account is no account found at the address.
func loadAccount(trie *muxdb.Trie, addr thor.Address, leafBank *muxdb.TrieLeafBank, steadyCommitNum uint32) (*Account, error) {
	data, meta, err := trie.FastGet(addr[:], leafBank, steadyCommitNum)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return emptyAccount(addr), nil
	}
	var a Account
	if err := rlp.DecodeBytes(data, &a); err != nil {
		return nil, err

	}
	if err := rlp.DecodeBytes(meta, &a.meta); err != nil {
		return nil, err
	}
	return &a, nil
}

// saveAccount save account into trie at given address.
// If the given account is empty, the value for given address is deleted.
func saveAccount(trie *muxdb.Trie, a *Account) error {
	if a.IsEmpty() {
		// delete if account is empty
		return trie.Update(a.meta.Addr[:], nil, nil)
	}

	data, err := rlp.EncodeToBytes(a)
	if err != nil {
		return err
	}

	meta, err := rlp.EncodeToBytes(&a.meta)
	if err != nil {
		return err
	}
	return trie.Update(a.meta.Addr[:], data, meta)
}

// loadStorage load storage data for given key.
func loadStorage(trie *muxdb.Trie, key thor.Bytes32, leafBank *muxdb.TrieLeafBank, steadyCommitNum uint32) (rlp.RawValue, error) {
	v, _, err := trie.FastGet(key[:], leafBank, steadyCommitNum)
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

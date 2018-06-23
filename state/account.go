// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

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
	return &Account{Balance: &big.Int{}, Energy: &big.Int{}}
}

// loadAccount load an account object by address in trie.
// It returns empty account is no account found at the address.
func loadAccount(trie trieReader, addr thor.Address) (*Account, error) {
	data, err := trie.TryGet(addr[:])
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
	return &a, nil
}

// saveAccount save account into trie at given address.
// If the given account is empty, the value for given address is deleted.
func saveAccount(trie trieWriter, addr thor.Address, a *Account) error {
	if a.IsEmpty() {
		// delete if account is empty
		return trie.TryDelete(addr[:])
	}

	data, err := rlp.EncodeToBytes(a)
	if err != nil {
		return err
	}
	return trie.TryUpdate(addr[:], data)
}

// loadStorage load storage data for given key.
func loadStorage(trie trieReader, key thor.Bytes32) (rlp.RawValue, error) {
	return trie.TryGet(key[:])
}

// saveStorage save value for given key.
// If the data is zero, the given key will be deleted.
func saveStorage(trie trieWriter, key thor.Bytes32, data rlp.RawValue) error {
	if len(data) == 0 {
		// release storage if data is zero length
		return trie.TryDelete(key[:])
	}
	return trie.TryUpdate(key[:], data)
}

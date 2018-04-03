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
	CodeHash    []byte // hash of code
	StorageRoot []byte // merkle root of the storage trie
}

// IsEmpty returns if an account is empty.
// An empty account has zero balance and zero length code hash.
func (a Account) IsEmpty() bool {
	return (a.Balance == nil || a.Balance.Sign() == 0) &&
		len(a.CodeHash) == 0
}

var emptyAccount = Account{Balance: &big.Int{}}

// loadAccount load an account object by address in trie.
// It returns empty account is no account found at the address.
func loadAccount(trie trieReader, addr thor.Address) (Account, error) {
	data, err := trie.TryGet(addr[:])
	if err != nil {
		return emptyAccount, err
	}
	if len(data) == 0 {
		return emptyAccount, nil
	}
	var a Account
	if err := rlp.DecodeBytes(data, &a); err != nil {
		return emptyAccount, err
	}
	return a, nil
}

// saveAccount save account into trie at given address.
// If the given account is empty, the value for given address is deleted.
func saveAccount(trie trieWriter, addr thor.Address, a Account) error {
	if a.IsEmpty() {
		// delete if account is empty
		return trie.TryDelete(addr[:])
	}

	data, err := rlp.EncodeToBytes(&a)
	if err != nil {
		return err
	}
	return trie.TryUpdate(addr[:], data)
}

// loadStorage load storage data for given key.
func loadStorage(trie trieReader, key thor.Bytes32) ([]byte, error) {
	return trie.TryGet(key[:])
}

// saveStorage save value for given key.
// If the data is zero, the given key will be deleted.
func saveStorage(trie trieWriter, key thor.Bytes32, data []byte) error {
	if len(data) == 0 {
		// release storage if data is zero length
		return trie.TryDelete(key[:])
	}
	return trie.TryUpdate(key[:], data)
}

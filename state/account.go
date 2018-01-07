package state

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
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
func loadAccount(trie trieReader, addr acc.Address) (Account, error) {
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
func saveAccount(trie trieWriter, addr acc.Address, a Account) error {
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

// loadStorage load storage value for given key.
func loadStorage(trie trieReader, key cry.Hash) (cry.Hash, error) {
	data, err := trie.TryGet(key[:])
	if err != nil {
		return cry.Hash{}, err
	}
	if len(data) == 0 {
		return cry.Hash{}, nil
	}

	_, content, _, err := rlp.Split(data)
	if err != nil {
		return cry.Hash{}, err
	}

	return cry.BytesToHash(content), nil
}

// saveStorage save value for given key.
// If the value is all zero, the given key will be deleted.
func saveStorage(trie trieWriter, key cry.Hash, value cry.Hash) error {
	if (cry.Hash{}) == value {
		// release storage is value is zero
		return trie.TryDelete(key[:])
	}

	trimed, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
	return trie.TryUpdate(key[:], trimed)
}

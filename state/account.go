// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"hash"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
	"github.com/vechain/thor/lowrlp"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/thor"
)

// AccountMetadata helps encode/decode account metadata.
type AccountMetadata []byte

// NewAccountMetadata builds the account metadata.
func NewAccountMetadata(storageInitCommitNum, storageCommitNum, storageDistinctNum uint32, addr thor.Address) AccountMetadata {
	w := lowrlp.NewEncoder()
	defer w.Release()

	w.EncodeUint(uint64(storageInitCommitNum))
	w.EncodeUint(uint64(storageCommitNum))
	w.EncodeUint(uint64(storageDistinctNum))
	w.EncodeString(addr[:])
	return w.ToBytes()
}

func (m AccountMetadata) split(i int) ([]byte, []byte) {
	var (
		content []byte
		rest    = m
		err     error
	)
	for ; i >= 0; i-- {
		if content, rest, err = rlp.SplitString(rest); err != nil {
			panic(errors.Wrap(err, "decode account metadata"))
		}
	}
	return content, rest
}

func (m AccountMetadata) splitUint32(i int) uint32 {
	c, _ := m.split(i)
	if len(c) > 4 { // 32-bit max
		panic(errors.New("decode account metadata: content too long"))
	}
	var n uint32
	for _, b := range c {
		n <<= 8
		n |= uint32(b)
	}
	return n
}

// StorageInitCommitNum returns the initial storage commit number.
func (m AccountMetadata) StorageInitCommitNum() uint32 {
	return m.splitUint32(0)
}

// StorageCommitNum returns the commit number of the last storage update.
func (m AccountMetadata) StorageCommitNum() uint32 {
	return m.splitUint32(1)
}

// StorageDistinctNum returns the distinct number of the last storage update.
func (m AccountMetadata) StorageDistinctNum() uint32 {
	return m.splitUint32(2)
}

// Address returns the account address.
func (m AccountMetadata) Address() (thor.Address, bool) {
	if n, err := rlp.CountValues(m); err != nil {
		panic(errors.Wrap(err, "decode account metadata"))
	} else if n == 4 {
		c, _ := m.split(3)
		if len(c) != 20 {
			panic(errors.New("decode account metadata: unexpected address length"))
		}
		return thor.BytesToAddress(c), true
	}
	return thor.Address{}, false
}

// SkipAddress returns the account metadata without address.
func (m AccountMetadata) SkipAddress() AccountMetadata {
	_, rest := m.split(2)
	return m[:len(m)-len(rest)]
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

	storageInitCommitNum, storageCommitNum, storageDistinctNum uint32
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
func loadAccount(trie *muxdb.Trie, addr thor.Address, steadyBlockNum uint32) (*Account, error) {
	h := hasherPool.Get().(*hasher)
	defer hasherPool.Put(h)

	data, meta, err := trie.FastGet(h.Hash(addr[:]), steadyBlockNum)
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
	a.storageInitCommitNum = am.StorageInitCommitNum()
	a.storageCommitNum = am.StorageCommitNum()
	a.storageDistinctNum = am.StorageDistinctNum()
	return &a, nil
}

// saveAccount save account into trie at given address.
// If the given account is empty, the value for given address is deleted.
func saveAccount(trie *muxdb.Trie, addr thor.Address, a *Account) error {
	h := hasherPool.Get().(*hasher)
	defer hasherPool.Put(h)

	if a.IsEmpty() {
		// delete if account is empty
		return trie.Update(h.Hash(addr[:]), nil, nil)
	}

	data, err := rlp.EncodeToBytes(a)
	if err != nil {
		return err
	}

	am := NewAccountMetadata(a.storageInitCommitNum, a.storageCommitNum, a.storageDistinctNum, addr)
	return trie.Update(h.Hash(addr[:]), data, am)
}

// loadStorage load storage data for given key.
func loadStorage(trie *muxdb.Trie, key thor.Bytes32, steadyBlockNum uint32) (rlp.RawValue, error) {
	h := hasherPool.Get().(*hasher)
	defer hasherPool.Put(h)

	v, _, err := trie.FastGet(h.Hash(key[:]), steadyBlockNum)
	return v, err
}

// saveStorage save value for given key.
// If the data is zero, the given key will be deleted.
func saveStorage(trie *muxdb.Trie, key thor.Bytes32, data rlp.RawValue) error {
	h := hasherPool.Get().(*hasher)
	defer hasherPool.Put(h)

	return trie.Update(
		h.Hash(key[:]),
		data,
		key[:], // key preimage as metadata
	)
}

type hasher struct {
	h   hash.Hash
	buf []byte
}

func (h *hasher) Hash(in []byte) []byte {
	if h.h == nil {
		h.h = thor.NewBlake2b()
	} else {
		h.h.Reset()
	}

	h.h.Write(in)
	h.buf = h.h.Sum(h.buf[:0])
	return h.buf
}

var hasherPool = sync.Pool{
	New: func() interface{} {
		return &hasher{}
	},
}

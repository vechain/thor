// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package state

import (
	"bytes"
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/thor"
)

// AccountMetadata is the account metadata.
type AccountMetadata struct {
	StorageID       []byte // the unique id of the storage trie.
	StorageMajorVer uint32 // the major version of the last storage update.
	StorageMinorVer uint32 // the minor version of the last storage update.
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
	// Nonce is activated at INTERSTELLAR by 0x02 txs. Encoded as an optional
	// trailing field by EncodeRLP so pre-fork accounts (Nonce==0) keep the
	// original 6-field layout — state root unchanged for existing nodes.
	// (`rlp:"optional"` tag not used: pinned go-ethereum v1.8.14 fork does
	// not support it, see `tx/reserved.go` for the same pattern.)
	Nonce uint64
}

// legacyAccount mirrors the pre-INTERSTELLAR 6-field layout. Used only as a
// wire format in EncodeRLP so Nonce==0 accounts round-trip to the historic
// encoding byte-for-byte.
type legacyAccount struct {
	Balance     *big.Int
	Energy      *big.Int
	BlockTime   uint64
	Master      []byte
	CodeHash    []byte
	StorageRoot []byte
}

// noncedAccount mirrors the post-INTERSTELLAR 7-field layout.
type noncedAccount struct {
	Balance     *big.Int
	Energy      *big.Int
	BlockTime   uint64
	Master      []byte
	CodeHash    []byte
	StorageRoot []byte
	Nonce       uint64
}

// EncodeRLP emits the legacy 6-field layout when Nonce==0 and the 7-field
// layout otherwise. This keeps state root stable for all historic accounts
// and for any post-INTERSTELLAR account that never participated in a 0x02 tx.
func (a *Account) EncodeRLP(w io.Writer) error {
	if a.Nonce == 0 {
		return rlp.Encode(w, &legacyAccount{
			Balance:     a.Balance,
			Energy:      a.Energy,
			BlockTime:   a.BlockTime,
			Master:      a.Master,
			CodeHash:    a.CodeHash,
			StorageRoot: a.StorageRoot,
		})
	}
	return rlp.Encode(w, &noncedAccount{
		Balance:     a.Balance,
		Energy:      a.Energy,
		BlockTime:   a.BlockTime,
		Master:      a.Master,
		CodeHash:    a.CodeHash,
		StorageRoot: a.StorageRoot,
		Nonce:       a.Nonce,
	})
}

// DecodeRLP accepts both the 6-field legacy layout (Nonce defaults to 0) and
// the 7-field nonced layout.
func (a *Account) DecodeRLP(s *rlp.Stream) error {
	var raws []rlp.RawValue
	if err := s.Decode(&raws); err != nil {
		return err
	}
	n := len(raws)
	if n != 6 && n != 7 {
		return fmt.Errorf("invalid account field count: %d", n)
	}
	if err := rlp.DecodeBytes(raws[0], &a.Balance); err != nil {
		return err
	}
	if err := rlp.DecodeBytes(raws[1], &a.Energy); err != nil {
		return err
	}
	if err := rlp.DecodeBytes(raws[2], &a.BlockTime); err != nil {
		return err
	}
	if err := rlp.DecodeBytes(raws[3], &a.Master); err != nil {
		return err
	}
	if err := rlp.DecodeBytes(raws[4], &a.CodeHash); err != nil {
		return err
	}
	if err := rlp.DecodeBytes(raws[5], &a.StorageRoot); err != nil {
		return err
	}
	if n == 7 {
		if err := rlp.DecodeBytes(raws[6], &a.Nonce); err != nil {
			return err
		}
	} else {
		a.Nonce = 0
	}
	return nil
}

// IsEmpty returns if an account is empty.
// An empty account has zero balance, zero energy, zero length master/codeHash,
// and zero nonce (EIP-161 alignment).
func (a *Account) IsEmpty() bool {
	return a.Balance.Sign() == 0 &&
		a.Energy.Sign() == 0 &&
		len(a.Master) == 0 &&
		len(a.CodeHash) == 0 &&
		a.Nonce == 0
}

var bigE18 = big.NewInt(1e18)

// CalcEnergy calculates energy based on current block time.
func (a *Account) CalcEnergy(blockTime uint64, stopTime uint64) *big.Int {
	if a.BlockTime == 0 {
		return a.Energy
	}

	if a.Balance.Sign() == 0 {
		return a.Energy
	}

	if blockTime <= a.BlockTime {
		return a.Energy
	}

	growth := new(big.Int)
	// If accounts last access block time is less than stop time, calculate energy growth.
	if a.BlockTime < stopTime {
		timeDiff := uint64(0)
		// if current block time is less than growth stop time, time diff is block time - account last access block time.
		// the same as before growth stop.
		if blockTime <= stopTime {
			timeDiff = blockTime - a.BlockTime
		} else {
			// if current block time is greater than growth stop time, we are taking the time diff only up to growth stop time.
			timeDiff = stopTime - a.BlockTime
		}
		// the rest of calculation is same as before growth stops.
		growth.SetUint64(timeDiff)
		growth.Mul(growth, a.Balance)
		growth.Mul(growth, thor.EnergyGrowthRate)
		growth.Div(growth, bigE18)
	}

	return new(big.Int).Add(a.Energy, growth)
}

func emptyAccount() *Account {
	a := Account{Balance: &big.Int{}, Energy: &big.Int{}}
	return &a
}

func secureKey(k []byte) []byte { return thor.Blake2b(k).Bytes() }

// loadAccount load an account object and its metadata by address in trie.
// It returns empty account is no account found at the address.
func loadAccount(trie *muxdb.Trie, addr thor.Address) (*Account, *AccountMetadata, error) {
	data, meta, err := trie.Get(secureKey(addr[:]))
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
		// delete if account is empty
		return trie.Update(secureKey(addr[:]), nil, nil)
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
	return trie.Update(secureKey(addr[:]), data, mdata)
}

// loadStorage load storage data for given key.
func loadStorage(trie *muxdb.Trie, key thor.Bytes32) (rlp.RawValue, error) {
	v, _, err := trie.Get(secureKey(key[:]))
	return v, err
}

// saveStorage save value for given key.
// If the data is zero, the given key will be deleted.
func saveStorage(trie *muxdb.Trie, key thor.Bytes32, data rlp.RawValue) error {
	return trie.Update(
		secureKey(key[:]),
		data,
		bytes.TrimLeft(key[:], "\x00"), // key preimage as metadata
	)
}

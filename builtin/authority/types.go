// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package authority

import (
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var (
	_ state.StorageEncoder = (*Entry)(nil)
	_ state.StorageDecoder = (*Entry)(nil)
	_ state.StorageEncoder = (*addressPtr)(nil)
	_ state.StorageDecoder = (*addressPtr)(nil)
)

// Entry contains all data of an authority entry.
type Entry struct {
	Endorsor thor.Address
	Identity thor.Bytes32
	Active   bool
	Prev     *thor.Address `rlp:"nil"`
	Next     *thor.Address `rlp:"nil"`
}

// Encode implements state.StorageEncoder.
func (e *Entry) Encode() ([]byte, error) {
	if e.IsEmpty() {
		return nil, nil
	}
	return rlp.EncodeToBytes(e)
}

// Decode implements state.StorageDecoder.
func (e *Entry) Decode(data []byte) error {
	if len(data) == 0 {
		*e = Entry{}
		return nil
	}
	return rlp.DecodeBytes(data, e)
}

// IsEmpty returns whether the entry can be treated as empty.
func (e *Entry) IsEmpty() bool {
	return e.Endorsor.IsZero() &&
		e.Identity.IsZero() &&
		!e.Active &&
		e.Prev == nil &&
		e.Next == nil
}

type addressPtr struct {
	Address *thor.Address `rlp:"nil"`
}

func (ap *addressPtr) Encode() ([]byte, error) {
	if ap.Address == nil {
		return nil, nil
	}
	return rlp.EncodeToBytes(&ap.Address)
}

func (ap *addressPtr) Decode(data []byte) error {
	if len(data) == 0 {
		ap.Address = nil
		return nil
	}
	return rlp.DecodeBytes(data, &ap.Address)
}

// Candidate candidate of block proposer.
type Candidate struct {
	Signer   thor.Address
	Endorsor thor.Address
	Identity thor.Bytes32
	Active   bool
}

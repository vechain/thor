// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package authority

import (
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

type (
	entry struct {
		Endorsor thor.Address
		Identity thor.Bytes32
		Active   bool
		Prev     *thor.Address `rlp:"nil"`
		Next     *thor.Address `rlp:"nil"`
	}

	addressPtr struct {
		Address *thor.Address `rlp:"nil"`
	}

	// Candidate candidate of block proposer.
	Candidate struct {
		Signer   thor.Address
		Endorsor thor.Address
		Identity thor.Bytes32
		Active   bool
	}
)

var (
	_ state.StorageEncoder = (*entry)(nil)
	_ state.StorageDecoder = (*entry)(nil)
	_ state.StorageEncoder = (*addressPtr)(nil)
	_ state.StorageDecoder = (*addressPtr)(nil)
)

// Encode implements state.StorageEncoder.
func (e *entry) Encode() ([]byte, error) {
	if e.IsEmpty() {
		return nil, nil
	}
	return rlp.EncodeToBytes(e)
}

// Decode implements state.StorageDecoder.
func (e *entry) Decode(data []byte) error {
	if len(data) == 0 {
		*e = entry{}
		return nil
	}
	return rlp.DecodeBytes(data, e)
}

// IsEmpty returns whether the entry can be treated as empty.
func (e *entry) IsEmpty() bool {
	return e.Endorsor.IsZero() &&
		e.Identity.IsZero() &&
		!e.Active &&
		e.Prev == nil &&
		e.Next == nil
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

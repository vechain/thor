// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/thor"
)

// Proposal is block Proposal.
type Proposal struct {
	ParentID  thor.Bytes32
	TxsRoot   thor.Bytes32
	GasLimit  uint64
	Timestamp uint64
	Signature []byte
	cache     struct {
		signer atomic.Value
		hash   atomic.Value
	}
}

// NewProposal creates a new Proposal.
func NewProposal(parentID, txsRoot thor.Bytes32, gasLimit, timestamp uint64) *Proposal {
	return &Proposal{
		ParentID:  parentID,
		TxsRoot:   txsRoot,
		GasLimit:  gasLimit,
		Timestamp: timestamp,
	}
}

// Number returns number of the Proposal.
func (d *Proposal) Number() uint32 {
	return Number(d.ParentID) + 1
}

// SigningHash returns the hash of the Proposal body without signature.
func (d *Proposal) SigningHash() (hash thor.Bytes32) {
	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b, d.GasLimit)
	binary.BigEndian.PutUint64(b[8:], d.Timestamp)

	// [parentID + txsRoot + Gaslimit + Timestamp]
	hash = thor.Blake2b(d.ParentID.Bytes(), d.TxsRoot.Bytes(), b)
	return
}

// Signer extract signer of the Proposal from signature.
func (d *Proposal) Signer() (signer thor.Address, err error) {
	if cached := d.cache.signer.Load(); cached != nil {
		return cached.(thor.Address), nil
	}
	defer func() {
		if err == nil {
			d.cache.signer.Store(signer)
		}
	}()

	pub, err := crypto.SigToPub(d.SigningHash().Bytes(), d.Signature)
	if err != nil {
		return thor.Address{}, err
	}

	signer = thor.Address(crypto.PubkeyToAddress(*pub))
	return
}

// WithSignature create a new Proposal with signature set.
func (d *Proposal) WithSignature(sig []byte) *Proposal {
	return &Proposal{
		ParentID:  d.ParentID,
		TxsRoot:   d.TxsRoot,
		GasLimit:  d.GasLimit,
		Timestamp: d.Timestamp,
		Signature: append([]byte(nil), sig...),
	}
}

// Hash is the hash of Proposal body.
func (d *Proposal) Hash() (hash thor.Bytes32) {
	if cached := d.cache.hash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { d.cache.hash.Store(hash) }()

	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b, d.GasLimit)
	binary.BigEndian.PutUint64(b[8:], d.Timestamp)

	// [parentID + txsRoot + gaslimit + timestamp + signature]
	hash = thor.Blake2b(d.ParentID.Bytes(), d.TxsRoot.Bytes(), b, d.Signature)
	return
}

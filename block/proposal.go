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

// Proposal is block proposal.
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

// NewProposal creates a new proposal.
func NewProposal(parentID, txsRoot thor.Bytes32, gasLimit, timestamp uint64) *Proposal {
	return &Proposal{
		ParentID:  parentID,
		TxsRoot:   txsRoot,
		GasLimit:  gasLimit,
		Timestamp: timestamp,
	}
}

// Number returns number of the proposal.
func (p *Proposal) Number() uint32 {
	return Number(p.ParentID) + 1
}

// SigningHash returns the hash of the proposal body without signature.
func (p *Proposal) SigningHash() (hash thor.Bytes32) {
	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b, p.GasLimit)
	binary.BigEndian.PutUint64(b[8:], p.Timestamp)

	// [parentID + txsRoot + Gaslimit + Timestamp]
	hash = thor.Blake2b(p.ParentID.Bytes(), p.TxsRoot.Bytes(), b)
	return
}

// Signer extract signer of the proposal from signature.
func (p *Proposal) Signer() (signer thor.Address, err error) {
	if cached := p.cache.signer.Load(); cached != nil {
		return cached.(thor.Address), nil
	}
	defer func() {
		if err == nil {
			p.cache.signer.Store(signer)
		}
	}()

	pub, err := crypto.SigToPub(p.SigningHash().Bytes(), p.Signature)
	if err != nil {
		return thor.Address{}, err
	}

	signer = thor.Address(crypto.PubkeyToAddress(*pub))
	return
}

// AsMessage returns the bytes of proposal body for being a message, which is parentID+txsRoot+gaslimit+timestamp+signer.
// Mostly used for computing backer signature.
func (p *Proposal) AsMessage(signer thor.Address) []byte {
	// [parentID + txsRoot + gaslimit + timestamp + signer]
	msg := make([]byte, 100)
	copy(msg[:], p.ParentID.Bytes())
	copy(msg[32:], p.TxsRoot.Bytes())
	binary.BigEndian.PutUint64(msg[64:], p.GasLimit)
	binary.BigEndian.PutUint64(msg[72:], p.Timestamp)
	copy(msg[80:], signer.Bytes())

	return msg
}

// WithSignature create a new proposal with signature set.
func (p *Proposal) WithSignature(sig []byte) *Proposal {
	return &Proposal{
		ParentID:  p.ParentID,
		TxsRoot:   p.TxsRoot,
		GasLimit:  p.GasLimit,
		Timestamp: p.Timestamp,
		Signature: append([]byte(nil), sig...),
	}
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"io"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

// Proposal is a block proposal
type Proposal struct {
	body struct {
		ParentID  thor.Bytes32
		TxsRoot   thor.Bytes32
		GasLimit  uint64
		Timestamp uint64
		Signature []byte
	}
	cache struct {
		signingHash atomic.Value
		signer      atomic.Value
		hash        atomic.Value
	}
}

// NewProposal creates a new proposal
func NewProposal(parentID, txsRoot thor.Bytes32, gasLimit, timestamp uint64) *Proposal {
	var p Proposal
	p.body.ParentID = parentID
	p.body.TxsRoot = txsRoot
	p.body.GasLimit = gasLimit
	p.body.Timestamp = timestamp

	return &p
}

// Number returns the number of the proposed block
func (p *Proposal) Number() uint32 {
	return Number(p.body.ParentID) + 1
}

// ParentID returns the parent block's ID
func (p *Proposal) ParentID() thor.Bytes32 {
	return p.body.ParentID
}

// Timestamp returns the unix timestamp of the proposed block
func (p *Proposal) Timestamp() uint64 {
	return p.body.Timestamp
}

// GasLimit returns the proposed gaslimit
func (p *Proposal) GasLimit() uint64 {
	return p.body.GasLimit
}

// TxsRoot returns the merkle root of proposed txs
func (p *Proposal) TxsRoot() thor.Bytes32 {
	return p.body.TxsRoot
}

// SigningHash returns the hash of the proposal body without signature
func (p *Proposal) SigningHash() (hash thor.Bytes32) {
	if cached := p.cache.signingHash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { p.cache.signingHash.Store(hash) }()

	hw := thor.NewBlake2b()
	rlp.Encode(hw, []interface{}{
		p.body.ParentID,
		p.body.TxsRoot,
		p.body.GasLimit,
		p.body.Timestamp,
	})
	hw.Sum(hash[:0])
	return
}

// Signer extract signer of the proposal from signature
func (p *Proposal) Signer() (signer thor.Address, err error) {
	if cached := p.cache.signer.Load(); cached != nil {
		return cached.(thor.Address), nil
	}
	defer func() {
		if err == nil {
			p.cache.signer.Store(signer)
		}
	}()

	pub, err := crypto.SigToPub(p.SigningHash().Bytes(), p.body.Signature)
	if err != nil {
		return thor.Address{}, err
	}

	signer = thor.Address(crypto.PubkeyToAddress(*pub))
	return
}

// WithSignature create a new proposal with signature set
func (p *Proposal) WithSignature(sig []byte) *Proposal {
	cpy := Proposal{body: p.body}
	cpy.body.Signature = append([]byte(nil), sig...)

	return &cpy
}

// Hash is the hash of RLP encoded proposal
func (p *Proposal) Hash() (hash thor.Bytes32) {
	if cached := p.cache.hash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { p.cache.hash.Store(hash) }()

	hw := thor.NewBlake2b()
	rlp.Encode(hw, p)
	hw.Sum(hash[:])
	return
}

// EncodeRLP implements rlp.Encoder
func (p *Proposal) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, p.body)
}

// DecodeRLP implements rlp.Decoder
func (p *Proposal) DecodeRLP(s *rlp.Stream) error {
	var body struct {
		ParentID  thor.Bytes32
		TxsRoot   thor.Bytes32
		GasLimit  uint64
		Timestamp uint64
		Signature []byte
	}

	if err := s.Decode(&body); err != nil {
		return err
	}
	*p = Proposal{body: body}
	return nil
}

package block

import (
	"io"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/vrf"
)

// Endorsement endorsement
type Endorsement struct {
	body endorsementBody

	cache struct {
		signer      atomic.Value
		signingHash atomic.Value
	}
}

type endorsementBody struct {
	BlockSummary *Summary
	VrfPublicKey *vrf.PublicKey
	VrfProof     *vrf.Proof

	Signature []byte
}

// NewEndorsement creates a new endorsement without signature
func NewEndorsement(bs *Summary, pubKey *vrf.PublicKey, pf *vrf.Proof) *Endorsement {
	return &Endorsement{
		body: endorsementBody{
			BlockSummary: bs,
		},
	}
}

// Copy copies the current endorsement
func (ed *Endorsement) Copy() *Endorsement {
	return &Endorsement{body: ed.body}
}

// Signer returns the signer
func (ed *Endorsement) Signer() (signer thor.Address, err error) {
	if cached := ed.cache.signer.Load(); cached != nil {
		return cached.(thor.Address), nil
	}
	defer func() {
		if err == nil {
			ed.cache.signer.Store(signer)
		}
	}()

	pub, err := crypto.SigToPub(ed.SigningHash().Bytes(), ed.body.Signature)
	if err != nil {
		return thor.Address{}, err
	}

	signer = thor.Address(crypto.PubkeyToAddress(*pub))
	return
}

// SigningHash computes the hash to be signed
func (ed *Endorsement) SigningHash() (hash thor.Bytes32) {
	if cached := ed.cache.signingHash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { ed.cache.signingHash.Store(hash) }()

	hw := thor.NewBlake2b()
	rlp.Encode(hw, []interface{}{
		ed.body.BlockSummary,
		ed.body.VrfPublicKey,
		ed.body.VrfProof,
	})
	hw.Sum(hash[:0])
	return
}

// WithSignature create a new Endorsement object with signature set.
func (ed *Endorsement) WithSignature(sig []byte) *Endorsement {
	cpy := Endorsement{body: ed.body}
	cpy.body.Signature = append([]byte(nil), sig...)

	return &cpy
}

// EncodeRLP implements rlp.Encoder
func (ed *Endorsement) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, ed.body)
}

// DecodeRLP implements rlp.Decoder.
func (ed *Endorsement) DecodeRLP(s *rlp.Stream) error {
	var body endorsementBody

	if err := s.Decode(&body); err != nil {
		return err
	}
	*ed = Endorsement{body: body}
	return nil
}

// BlockSummary returns the block summary
func (ed *Endorsement) BlockSummary() *Summary {
	return ed.body.BlockSummary
}

// VrfPublicKey returns the VRF public key
func (ed *Endorsement) VrfPublicKey() *vrf.PublicKey {
	return ed.body.VrfPublicKey
}

// VrfProof returns the VRF proof
func (ed *Endorsement) VrfProof() *vrf.Proof {
	return ed.body.VrfProof
}

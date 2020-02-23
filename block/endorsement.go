package block

import (
	"fmt"
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
		id          atomic.Value
	}
}

type endorsementBody struct {
	BlockSummary *Summary
	// VrfPublicKey *vrf.PublicKey
	VrfProof *vrf.Proof

	Signature []byte
}

// NewEndorsement creates a new endorsement without signature
func NewEndorsement(bs *Summary, proof *vrf.Proof) *Endorsement {
	return &Endorsement{
		body: endorsementBody{
			BlockSummary: bs.Copy(),
			// VrfPublicKey: pubKey,
			VrfProof: proof.Copy(),
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
	return rlp.Encode(w, &ed.body)
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
	return ed.body.BlockSummary.Copy()
}

// VrfProof returns the VRF proof
func (ed *Endorsement) VrfProof() *vrf.Proof {
	return ed.body.VrfProof.Copy()
}

// Signature returns the signature
func (ed *Endorsement) Signature() []byte {
	return append([]byte(nil), ed.body.Signature...)
}

func (ed *Endorsement) String() string {
	var signerStr string
	if signer, err := ed.Signer(); err != nil {
		signerStr = "N/A"
	} else {
		signerStr = signer.String()
	}

	s := fmt.Sprintf(`Endorsement(%v):
	BlockSummary:   	%v
	Signer:         	%v
	VrfProof:         	0x%x
	Signature:      	0x%x
	`, ed.SigningHash(), ed.body.BlockSummary.ID(), signerStr, ed.body.VrfProof, ed.body.Signature)

	return s
}

// ID computes the endorsement ID. ID = hash(signing_hash || signer)
func (ed *Endorsement) ID() (id thor.Bytes32) {
	if cached := ed.cache.id.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}
	defer func() { ed.cache.id.Store(id) }()

	signer, err := ed.Signer()
	if err != nil {
		return
	}

	hw := thor.NewBlake2b()
	hw.Write(ed.SigningHash().Bytes())
	hw.Write(signer.Bytes())
	hw.Sum(id[:0])
	return
}

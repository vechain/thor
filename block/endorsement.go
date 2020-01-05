package block

import "github.com/vechain/thor/vrf"

// Endorsement endorsement
type Endorsement struct {
	body endorsementBody
	
	cache struct {
		signer atomic.Value
		signingHash      atomic.Value
	}
}

type endorsementBody struct {
	BlockSummary *Summary
	VrfPublickey    *vrf.PublicKey
	VrfProof     *vrf.Proof

	Signature []byte
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

	pub, err := crypto.SigToPub(ed.SigningHash().Bytes(), ed.Signature)
	if err != nil {
		return thor.Address{}, err
	}

	signer = thor.Address(crypto.PubkeyToAddress(*pub))
	return
}

// SigniningHash computes the hash to be signed
func (ed *Endorsement) SigniningHash() (hash thor.Bytes32) {
	if cached := ed.cache.signingHash.Load(); cached != nil {
		return cached.(thor.Bytes32)
	}	
	defer func() { ed.cache.signingHash.Store(hash) } ()
	
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
	return rlp.Encode(hw, ed.body)
}

// DecodeRLP implements rlp.Decoder.
func (ed *Endorsement) DecodeRLP(s *rlp.Stream) error {
	var body endorsementBody

	if err := s.Decode(&bodyValue); err != nil {
		return err
	}
	*ed = Endorsement{body: body}
	return nil
}

// Endorsements endorsement array
type Endorsements []*Endorsement

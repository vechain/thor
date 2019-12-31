package vrf

import (
	"errors"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/protocol"
)

// constants
const (
	PrivateKeyLen = 64
	PublicKeyLen  = 32
	ProofLen      = 80
	HashLen       = 64
)

// PrivateKey VRF private key
type PrivateKey [PrivateKeyLen]byte

// PublicKey VRF public key
type PublicKey [PublicKeyLen]byte

// Proof VRF proof
type Proof [ProofLen]byte

// Hash VRF hash
type Hash [HashLen]byte

// GenKeyPair generates a VRF key pair
func GenKeyPair() (*PublicKey, *PrivateKey) {
	_pk, _sk := crypto.VrfKeygen()

	var (
		pk PublicKey
		sk PrivateKey
	)
	copy(pk[:], _pk[:])
	copy(sk[:], _sk[:])

	return &pk, &sk
}

// PublicKey computes VRF public key from VRF private key
func (k *PrivateKey) PublicKey() *PublicKey {
	var sk crypto.VrfPrivkey
	copy(sk[:], k[:])

	_pk := sk.Pubkey()
	var pk PublicKey
	copy(pk[:], _pk[:])
	return &pk
}

// Prove computes VRF proof for given message
func (k *PrivateKey) Prove(msg []byte) (*Proof, error) {
	var sk crypto.VrfPrivkey
	copy(sk[:], k[:])

	pf, ok := sk.Prove(hashable(msg))

	if !ok {
		return nil, errors.New("vrf *PrivateKey Prove ")
	}

	var proof Proof
	copy(proof[:], pf[:])
	return &proof, nil
}

// Verify verifies VRF proof and outputs the corresponding hash
func (k *PublicKey) Verify(proof *Proof, msg []byte) (*Hash, error) {
	var pk crypto.VrfPubkey
	copy(pk[:], k[:])

	var pf crypto.VrfProof
	copy(pf[:], proof[:])

	ok, h := pk.Verify(pf, hashable(msg))
	if !ok {
		return nil, errors.New("vrf *PublicKey Verify")
	}

	var hash Hash
	copy(hash[:], h[:])
	return &hash, nil
}

// Hash computes hash from VRF proof
func (p *Proof) Hash() (*Hash, error) {
	var pf crypto.VrfProof
	copy(pf[:], p[:])

	h, ok := pf.Hash()
	if !ok {
		return nil, errors.New("vrf *Proof Hash")
	}

	var hash Hash
	copy(hash[:], h[:])
	return &hash, nil
}

type hashable []byte

func (h hashable) ToBeHashed() (protocol.HashID, []byte) {
	return protocol.HashID(""), h[:]
}

package vrf

import (
	"errors"
	"io"

	"github.com/algorand/go-algorand/crypto"
	"github.com/algorand/go-algorand/protocol"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
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

// GenKeyPairFromSeed ...
func GenKeyPairFromSeed(seed []byte) (*PublicKey, *PrivateKey) {
	var s [32]byte
	nbyte := copy(s[:], seed)
	if nbyte < 32 {
		log.Debug("Number of byte copied: %d", nbyte)
	}

	_pk, _sk := crypto.VrfKeygenFromSeed(s)

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
func (k *PublicKey) Verify(proof *Proof, msg []byte) (bool, *Hash) {
	var pk crypto.VrfPubkey
	copy(pk[:], k[:])

	var pf crypto.VrfProof
	copy(pf[:], proof[:])

	ok, h := pk.Verify(pf, hashable(msg))
	if !ok {
		return false, nil
	}

	var hash Hash
	copy(hash[:], h[:])
	return true, &hash
}

// Bytes32 returns the public key as thor.Bytes32
func (k *PublicKey) Bytes32() thor.Bytes32 {
	return thor.BytesToBytes32(k[:])
}

// Copy returns a VRF proof copy
func (p *Proof) Copy() *Proof {
	pf := *p
	return &pf
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

// Bytes returns the proof as a byte array
func (p *Proof) Bytes() []byte {
	return append([]byte(nil), p[:]...)
}

// EncodeRLP peroforms rlp encoding
func (p *Proof) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, p.Bytes())
}

// DecodeRLP performs rlp decoding
func (p *Proof) DecodeRLP(s *rlp.Stream) error {
	var b []byte

	if err := s.Decode(&b); err != nil {
		return err
	}
	copy(p[:], b)
	return nil
}

type hashable []byte

func (h hashable) ToBeHashed() (protocol.HashID, []byte) {
	return protocol.HashID(""), h[:]
}

// // Proofs defines an array of VRF proofs
// type Proofs []*Proof

// func (pfs Proofs) Len() int      { return len(pfs) }
// func (pfs Proofs) Swap(i, j int) { pfs[i], pfs[j] = pfs[j], pfs[i] }
// func (pfs Proofs) Less(i, j int) bool {
// 	for n := 0; n < ProofLen; n++ {
// 		if pfs[i][n] < pfs[j][n] {
// 			return true
// 		} else if pfs[i][n] > pfs[j][n] {
// 			return false
// 		}
// 	}

// 	return false
// }

// func (pfs Proofs) String() string {
// 	var str string
// 	for i := 0; i < len(pfs); i++ {
// 		str += fmt.Sprintf("%d %x\n", i, pfs[i])
// 	}

// 	return str
// }

// BytesToProof ...
func BytesToProof(b []byte) *Proof {
	var p Proof

	if len(b) > len(p) {
		b = b[len(b)-ProofLen:]
	}

	copy(p[ProofLen-len(b):], b)

	return &p
}

// Bytes32ToPublicKey converts Byte32 into a vrf public key
func Bytes32ToPublicKey(b thor.Bytes32) *PublicKey {
	var pk PublicKey
	copy(pk[:], b.Bytes())
	return &pk
}

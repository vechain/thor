package cry

import (
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/thor"
)

var (
	signerCacheSize = 1024
)

// Signable interface of signable object.
type Signable interface {
	SigningHash() thor.Hash
	Signature() []byte

	Hash() thor.Hash
}

// Signing to sign a signable object or extract signer.
type Signing struct {
	genesisHash thor.Hash
	cache       *cache.LRU
}

// NewSigning create a signing object.
// The 'genesisHash' is to prevent cross-chain replay attack.
func NewSigning(genesisHash thor.Hash) *Signing {
	return &Signing{
		genesisHash,
		cache.NewLRU(signerCacheSize),
	}
}

// xor signing hash with genesis hash
func (s *Signing) maskHash(signingHash *thor.Hash) {
	for i := range signingHash {
		signingHash[i] ^= s.genesisHash[i]
	}
}

// Sign sign the target with given private key.
func (s *Signing) Sign(target Signable, privateKey []byte) ([]byte, error) {
	signingHash := target.SigningHash()
	s.maskHash(&signingHash)

	priv, err := crypto.ToECDSA(privateKey)
	if err != nil {
		return nil, err
	}
	return crypto.Sign(signingHash[:], priv)
}

// Signer extract signer from signed target.
func (s *Signing) Signer(target Signable) (thor.Address, error) {
	hash := target.Hash()
	if addr, ok := s.cache.Get(hash); ok {
		return addr.(thor.Address), nil
	}

	signingHash := target.SigningHash()
	s.maskHash(&signingHash)

	pub, err := crypto.SigToPub(signingHash[:], target.Signature())
	if err != nil {
		return thor.Address{}, err
	}
	addr := thor.Address(crypto.PubkeyToAddress(*pub))
	s.cache.Add(hash, addr)
	return addr, nil
}

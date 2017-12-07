package dsa

import (
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/acc"
	"github.com/vechain/thor/cry"
)

// TODO: consider adding genesis hash

// Signer extracts signer.
func Signer(msgHash cry.Hash, sig []byte) (*acc.Address, error) {
	pub, err := crypto.SigToPub(msgHash[:], sig)
	if err != nil {
		return nil, err
	}
	addr := crypto.PubkeyToAddress(*pub)
	return (*acc.Address)(&addr), nil
}

// Sign sign a signable message.
func Sign(msgHash cry.Hash, privateKey []byte) ([]byte, error) {
	priv, err := crypto.ToECDSA(privateKey)
	if err != nil {
		return nil, err
	}

	return crypto.Sign(msgHash[:], priv)
}

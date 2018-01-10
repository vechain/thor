package dsa

import (
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/thor"
)

// TODO: consider adding genesis hash

// Signer extracts signer.
func Signer(msgHash thor.Hash, sig []byte) (thor.Address, error) {
	pub, err := crypto.SigToPub(msgHash[:], sig)
	if err != nil {
		return thor.Address{}, err
	}
	addr := crypto.PubkeyToAddress(*pub)
	return thor.Address(addr), nil
}

// Sign sign a signable message.
func Sign(msgHash thor.Hash, privateKey []byte) ([]byte, error) {
	priv, err := crypto.ToECDSA(privateKey)
	if err != nil {
		return nil, err
	}

	return crypto.Sign(msgHash[:], priv)
}

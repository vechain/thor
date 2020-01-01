package vrf

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestPublicKey(t *testing.T) {
	pk, sk := GenKeyPair()

	_pk := sk.PublicKey()

	if bytes.Compare(pk[:], _pk[:]) != 0 {
		t.Errorf("Test failed")
	}

	if bytes.Compare(pk[:], sk[32:]) != 0 {
		t.Errorf("Test failed")
	}
}

func TestVrfFunc(t *testing.T) {
	pk, sk := GenKeyPair()

	msg := []byte("test")

	var (
		proof       *Proof
		hash, _hash *Hash
		err         error
	)

	proof, err = sk.Prove(msg)
	if err != nil {
		t.Error(err)
	}

	hash, err = pk.Verify(proof, msg)
	if err != nil {
		t.Error(err)
	}

	_hash, err = proof.Hash()
	if err != nil {
		t.Error(err)
	}

	if bytes.Compare(hash[:], _hash[:]) != 0 {
		t.Errorf("Test failed")
	}
}

func BenchmarkVrfKeyGen(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenKeyPair()
	}
}

func BenchmarkVrfProve(b *testing.B) {
	_, sk := GenKeyPair()
	msg := make([]byte, 32)

	for i := 0; i < b.N; i++ {
		rand.Read(msg)
		sk.Prove(msg)
	}
}

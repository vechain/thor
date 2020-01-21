package vrf

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"sort"
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

func TestVrfProofBytes(t *testing.T) {
	_, sk := GenKeyPair()

	proof, _ := sk.Prove([]byte("message"))
	b := proof.Bytes()

	if bytes.Compare(proof[:], b) != 0 {
		t.Errorf("Byte values not equal")
	}

	b[0] = b[0] + 1
	if bytes.Compare(proof[:], b) == 0 {
		t.Errorf("Byte() returns a reference, not a new []byte")
	}
}

func TestVrf(t *testing.T) {
	pk, sk := GenKeyPair()
	msg := []byte("PositiveMsg")
	_msg := []byte("NegativeMsg")

	// pf, _ := sk.Prove([]byte(nil))
	// fmt.Println(hex.EncodeToString(pf[:]))

	proof, err := sk.Prove(msg)
	if err != nil {
		t.Fatal(err)
	}

	if ok, _ := pk.Verify(proof, msg); !ok {
		t.Errorf("Verification failed")
	}

	if ok, _ := pk.Verify(proof, _msg); ok {
		t.Errorf("Verification failed")
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

func BenchmarkVrfProveVerify(b *testing.B) {
	pk, sk := GenKeyPair()
	msg := make([]byte, 32)

	for i := 0; i < b.N; i++ {
		rand.Read(msg)
		proof, _ := sk.Prove(msg)
		pk.Verify(proof, msg)
	}
}

func TestProofCompare(t *testing.T) {
	pArray := make([]*Proof, 10)
	var b [ProofLen]byte

	for i := 0; i < cap(pArray); i++ {
		c, _ := rand.Read(b[:])
		if c != ProofLen {
			t.Errorf("")
		}

		pf := Proof(b)
		pArray[i] = &pf
	}

	var proofs = Proofs(pArray)
	fmt.Println(proofs.String())
	sort.Sort(proofs)
	fmt.Println(proofs.String())

	return
}

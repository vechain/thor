package node

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestDeriveVrfPrivateKey(t *testing.T) {
	sk, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	master := Master{
		PrivateKey: sk,
	}

	master.deriveVrfPrivateKey()

	secret := master.PrivateKey.D.Bytes()
	if bytes.Compare(secret, master.VrfPrivateKey[:32]) != 0 {
		t.Errorf("Test failed")
	}
}

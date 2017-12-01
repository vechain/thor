package cry

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var address_0 = common.FromHex("bd770416a3345f91e4b34576cb804a576fa48eb1")

func TestCreateAddress(t *testing.T) {

	add_want := common.BytesToAddress([]byte{})
	v := crypto.CreateAddress(add_want, 0)
	vb := AddressToBytes(v)
	vw := common.FromHex("bd770416a3345f91e4b34576cb804a576fa48eb1")

	if !bytes.Equal(vb, vw) {
		t.Fatalf("empty hash mismatch: want: %x have: %x", vw, vb)

	}
}

func TestEcdsa(t *testing.T) {

	priv_origin, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("error generating private key")
	}

	address_origin := crypto.PubkeyToAddress(priv_origin.PublicKey)

	fmt.Printf("origin address is 0x%x\n", address_origin)

	priv_second, _ := crypto.ToECDSA(crypto.FromECDSA(priv_origin))
	address_second := crypto.PubkeyToAddress(priv_second.PublicKey)
	fmt.Printf("second address is 0x%x\n", address_second)

}

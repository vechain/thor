package cry

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
)

var testAddrHex = "970e8128ab834e8eac17ab8e3812f010678cf791"
var testPrivHex = "289c2857d4598e37fb9647507e47a309d6133539bf21a8b9cb6df88fd5232032"

func TestSign(t *testing.T) {
	key, _ := HexToECDSA(testPrivHex)
	addr2 := PubkeyToAddress(key.PublicKey)
	fmt.Printf("addr2=0x%x\n", addr2)
	addr := common.HexToAddress(testAddrHex)

	msg := Keccak256([]byte("foo"))
	fmt.Printf("msg=%x\n", msg)
	sig, err := Sign(msg, key)
	if err != nil {
		t.Errorf("Sign error: %s", err)
	}
	recoveredPub, err := Ecrecover(msg, sig)
	fmt.Printf("pubkey=0x%x\n", recoveredPub)
	if err != nil {
		t.Errorf("ECRecover error: %s", err)
	}
	pubKey := ToECDSAPub(recoveredPub)
	recoveredAddr := PubkeyToAddress(*pubKey)
	if addr != recoveredAddr {
		t.Errorf("Address mismatch: want: %x have: %x", addr, recoveredAddr)
	}

	// should be equal to SigToPub
	recoveredPub2, err := SigToPub(msg, sig)
	add3 := PubkeyToAddress(*recoveredPub2)
	fmt.Printf("add3=%x", add3)
	if err != nil {
		t.Errorf("ECRecover error: %s", err)
	}
	recoveredAddr2 := PubkeyToAddress(*recoveredPub2)
	if addr != recoveredAddr2 {
		t.Errorf("Address mismatch: want: %x have: %x", addr, recoveredAddr2)
	}
}

//19457468657265756D205369676E6564204D6573736167653A0A3332387a8233c96e1fc0ad5e284353276177af2186e7afa85296f106336e376669f7
func TestSignature(t *testing.T) {
	priv := "356e9bc0848b9c1ed1ea75b17901e0dd64bfefe33626be8ca4376b48ad9a3564"

	rawdata := make([]byte, 0)
	txraw := common.FromHex("ea108504a817c80083015f9094c5f36811c884cbd54cfafcbbe51f28fd641c6787880de0b6b3a764000080")
	txrawhash := Keccak256(txraw)
	fmt.Printf("txrawhash=%x\n\n", txrawhash)

	//	msg := common.FromHex("0x10665400afc19fd64baafb83c129133f4b11e55f51394fd0184eb1974fa00411")
	prefix := common.FromHex("19457468657265756d205369676e6564204d6573736167653a0a3332")
	rawdata = append(rawdata, prefix...)
	rawdata = append(rawdata, txrawhash...)
	tx2sign := Keccak256(rawdata)
	fmt.Printf("rawdata=%x\nmsg=%x\n", rawdata[:], tx2sign)
	privatekey := ToECDSA(common.FromHex(priv))
	si, _ := Sign(tx2sign, privatekey)
	fmt.Printf("mysig=%x\n\n", si)

	//hash := common.FromHex("0x387a8233c96e1fc0ad5e284353276177af2186e7afa85296f106336e376669f7")
	strsig := "0x45630b849f2d57a29c33081a45eca7d287979247d3580d2450a1e7f635752b5b4741d89e7702b7ae4b37f6657c1da3e2d0680c9f0bbf6d421c410944bcca6abf1c"
	sig := common.FromHex(strsig)
	sig[64] -= 0x1b
	fmt.Printf("sig=%x\n", sig)
	hashmsg := common.FromHex("0xb13025dad68717da0f89981a155dbbd1b97f2e421a85967cecceb4c33e81bbcd")
	fmt.Printf("hash_msg=%x\n", hashmsg)
	recoveredPub, _ := Ecrecover(hashmsg, sig)
	pub := ToECDSAPub(recoveredPub)
	fmt.Printf("pubkey=%x\n", pub)
	pubkey := PubkeyToAddress(*pub)
	fmt.Printf("pubkey=%x\n", pubkey)

	r, _ := new(big.Int).SetString("45630b849f2d57a29c33081a45eca7d287979247d3580d2450a1e7f635752b5b", 16)
	s, _ := new(big.Int).SetString("4741d89e7702b7ae4b37f6657c1da3e2d0680c9f0bbf6d421c410944bcca6abf", 16)
	bs := new(big.Int).Sub(secp256k1.N, s)

	rr := make([]byte, 0, 65)
	rr = append(rr, r.Bytes()...)
	rr = append(rr, bs.Bytes()...)
	if sig[64] == 0x0 {
		sig[64] = 1
	} else {
		sig[64] = 0
	}

	rr = append(rr, sig[64])
	fmt.Printf("rr=%x\n", rr)

	fmt.Printf("r=%x\n", r)
	fmt.Printf("s=%x\n", s)
	fmt.Printf("bs=%x\n", bs)

	mrecoveredPub, _ := Ecrecover(hashmsg, rr)
	mpub := ToECDSAPub(mrecoveredPub)
	fmt.Printf("mpubkey=%x\n", mpub)
	mpubkey := PubkeyToAddress(*mpub)
	fmt.Printf("mpubkey=%x\n", mpubkey)
}

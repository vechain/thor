package main_test

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/vechain/thor/abi"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

type account struct {
	Address    thor.Address
	PrivateKey *ecdsa.PrivateKey
}

var accounts []account
var privKeys = []string{
	"dce1443bd2ef0c2631adc1c67e5c93f13dc23a41c18b536effbbdcbcdb96fb65",
	"321d6443bc6177273b5abf54210fe806d451d6b7973bccc2384ef78bbcd0bf51",
	"2d7c882bad2a01105e36dda3646693bc1aaaa45b0ed63fb0ce23c060294f3af2",
	"593537225b037191d322c3b1df585fb1e5100811b71a6f7fc7e29cca1333483e",
	"ca7b25fc980c759df5f3ce17a3d881d6e19a38e651fc4315fc08917edab41058",
	"88d2d80b12b92feaa0da6d62309463d20408157723f2d7e799b6a74ead9a673b",
	"fbb9e7ba5fe9969a71c6599052237b91adeb1e5fc0c96727b66e56ff5d02f9d0",
	"547fb081e73dc2e22b4aae5c60e2970b008ac4fc3073aebc27d41ace9c4f53e9",
	"c8c53657e41a8d669349fc287f57457bd746cb1fcfc38cf94d235deb2cfca81b",
	"87e0eba9c86c494d98353800571089f316740b0cb84c9a7cdf2fe5c9997c7966",
}
var nonce uint64 = uint64(time.Now().UnixNano())

func initAccounts(t *testing.T) {
	for _, str := range privKeys {
		pk, err := crypto.HexToECDSA(str)
		if err != nil {
			t.Error(err)
		}
		addr := crypto.PubkeyToAddress(pk.PublicKey)
		accounts = append(accounts, account{thor.Address(addr), pk})
	}
}

func TestNormalTransaction(t *testing.T) {
	initAccounts(t)

	tx := new(tx.Builder).
		ChainTag(0x50).
		BlockRef(tx.NewBlockRef(0)).
		Expiration(math.MaxUint32).
		Clause(tx.NewClause(&accounts[1].Address).WithValue(big.NewInt(100))).
		Gas(300000).GasPriceCoef(0).Nonce(nonce).Build()

	sig, err := crypto.Sign(tx.SigningHash().Bytes(), accounts[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	tx = tx.WithSignature(sig)

	var txRLP bytes.Buffer
	tx.EncodeRLP(&txRLP)

	t.Log(hex.EncodeToString(txRLP.Bytes()))
}

func TestEnergyTransaction(t *testing.T) {
	initAccounts(t)

	tx := new(tx.Builder).
		ChainTag(0x50).
		BlockRef(tx.NewBlockRef(0)).
		Expiration(math.MaxUint32).
		Clause(tx.NewClause(&builtin.Energy.Address).
			WithData(mustEncodeInput(builtin.Energy.ABI, "transfer", accounts[1].Address, big.NewInt(100)))).
		Gas(300000).GasPriceCoef(0).Nonce(nonce).Build()

	sig, err := crypto.Sign(tx.SigningHash().Bytes(), accounts[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	tx = tx.WithSignature(sig)

	var txRLP bytes.Buffer
	tx.EncodeRLP(&txRLP)

	t.Log(hex.EncodeToString(txRLP.Bytes()))
}

func TestMultiClause(t *testing.T) {
	initAccounts(t)

	tx := new(tx.Builder).
		ChainTag(0x50).
		Clause(tx.NewClause(&accounts[1].Address).WithValue(big.NewInt(100))).
		Clause(tx.NewClause(&accounts[2].Address).WithValue(big.NewInt(100))).
		Gas(300000).GasPriceCoef(0).Nonce(nonce).Build()

	sig, err := crypto.Sign(tx.SigningHash().Bytes(), accounts[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	tx = tx.WithSignature(sig)
	t.Log(tx.String())
}

func TestGetThorBalance(t *testing.T) {
	initAccounts(t)

	txData := mustEncodeInput(builtin.Energy.ABI, "balanceOf", accounts[0].Address)
	t.Log(builtin.Energy.Address)
	t.Log(fmt.Sprintf("%x", txData))
}

func mustEncodeInput(abi *abi.ABI, name string, args ...interface{}) []byte {
	m, ok := abi.MethodByName(name)
	if !ok {
		panic("no method '" + name + "'")
	}
	data, err := m.EncodeInput(args...)
	if err != nil {
		panic(err)
	}
	return data
}

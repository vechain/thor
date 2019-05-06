// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx_test

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
)

func TestTx(t *testing.T) {
	to, _ := thor.ParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed")
	trx := new(tx.Builder).ChainTag(1).
		BlockRef(tx.BlockRef{0, 0, 0, 0, 0xaa, 0xbb, 0xcc, 0xdd}).
		Expiration(32).
		Clause(tx.NewClause(&to).WithValue(big.NewInt(10000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
		Clause(tx.NewClause(&to).WithValue(big.NewInt(20000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
		GasPriceCoef(128).
		Gas(21000).
		DependsOn(nil).
		Nonce(12345678).Build()

	assert.Equal(t, "0x2a1c25ce0d66f45276a5f308b99bf410e2fc7d5b6ea37a49f2ab9f1da9446478", trx.SigningHash().String())
	assert.Equal(t, thor.Bytes32{}, trx.ID())

	assert.Equal(t, uint64(21000), func() uint64 { g, _ := new(tx.Builder).Build().IntrinsicGas(); return g }())
	assert.Equal(t, uint64(37432), func() uint64 { g, _ := trx.IntrinsicGas(); return g }())

	assert.Equal(t, big.NewInt(150), trx.GasPrice(big.NewInt(100)))
	assert.Equal(t, []byte(nil), trx.Signature())

	assert.Equal(t, false, trx.Features().IsDelegated())

	delegator, _ := trx.Delegator()
	assert.Nil(t, delegator)

	k, _ := hex.DecodeString("7582be841ca040aa940fff6c05773129e135623e41acce3e0b8ba520dc1ae26a")
	priv, _ := crypto.ToECDSA(k)
	sig, _ := crypto.Sign(trx.SigningHash().Bytes(), priv)

	trx = trx.WithSignature(sig)
	assert.Equal(t, "0xd989829d88b0ed1b06edf5c50174ecfa64f14a64", func() string { s, _ := trx.Origin(); return s.String() }())
	assert.Equal(t, "0xda90eaea52980bc4bb8d40cb2ff84d78433b3b4a6e7d50b75736c5e3e77b71ec", trx.ID().String())

	assert.Equal(t, "f8970184aabbccdd20f840df947567d83b7b8d80addcb281a71d54fc7b3364ffed82271086000000606060df947567d83b7b8d80addcb281a71d54fc7b3364ffed824e208600000060606081808252088083bc614ec0b841f76f3c91a834165872aa9464fc55b03a13f46ea8d3b858e528fcceaf371ad6884193c3f313ff8effbb57fe4d1adc13dceb933bedbf9dbb528d2936203d5511df00",
		func() string { d, _ := rlp.EncodeToBytes(trx); return hex.EncodeToString(d) }(),
	)

}

func TestDelegatedTx(t *testing.T) {
	to, _ := thor.ParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed")
	origin, _ := hex.DecodeString("7582be841ca040aa940fff6c05773129e135623e41acce3e0b8ba520dc1ae26a")
	delegator, _ := hex.DecodeString("321d6443bc6177273b5abf54210fe806d451d6b7973bccc2384ef78bbcd0bf51")

	var feat tx.Features
	feat.SetDelegated(true)

	trx := new(tx.Builder).ChainTag(0xa4).
		BlockRef(tx.BlockRef{0, 0, 0, 0, 0xaa, 0xbb, 0xcc, 0xdd}).
		Expiration(32).
		Clause(tx.NewClause(&to).WithValue(big.NewInt(10000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
		Clause(tx.NewClause(&to).WithValue(big.NewInt(20000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
		GasPriceCoef(128).
		Gas(210000).
		DependsOn(nil).
		Features(feat).
		Nonce(12345678).Build()

	assert.Equal(t, "0x96c4cd08584994f337946f950eca5511abe15b152bc879bf47c2227901f9f2af", trx.SigningHash().String())
	assert.Equal(t, true, trx.Features().IsDelegated())

	p1, _ := crypto.ToECDSA(origin)
	sig, _ := crypto.Sign(trx.SigningHash().Bytes(), p1)

	o := crypto.PubkeyToAddress(p1.PublicKey)
	hash := trx.DelegatorSigningHash(thor.Address(o))
	p2, _ := crypto.ToECDSA(delegator)
	delegatorSig, _ := crypto.Sign(hash.Bytes(), p2)

	sig = append(sig, delegatorSig...)
	trx = trx.WithSignature(sig)

	assert.Equal(t, "0x956577b09b2a770d10ea129b26d916955df3606dc973da0043d6321b922fdef9", hash.String())
	assert.Equal(t, "0xd989829d88b0ed1b06edf5c50174ecfa64f14a64", func() string { s, _ := trx.Origin(); return s.String() }())
	assert.Equal(t, "0x956577b09b2a770d10ea129b26d916955df3606dc973da0043d6321b922fdef9", trx.ID().String())
	assert.Equal(t, "0xd3ae78222beadb038203be21ed5ce7c9b1bff602", func() string { s, _ := trx.Delegator(); return s.String() }())

	assert.Equal(t, "f8db81a484aabbccdd20f840df947567d83b7b8d80addcb281a71d54fc7b3364ffed82271086000000606060df947567d83b7b8d80addcb281a71d54fc7b3364ffed824e20860000006060608180830334508083bc614ec101b882bad4d4401b1fb1c41d61727d7fd2aeb2bb3e65a27638a5326ca98404c0209ab159eaeb37f0ac75ed1ac44d92c3d17402d7d64b4c09664ae2698e1102448040c000f043fafeaf60343248a37e4f1d2743b4ab9116df6d627b4d8a874e4f48d3ae671c4e8d136eb87c544bea1763673a5f1762c2266364d1b22166d16e3872b5a9c700",
		func() string { d, _ := rlp.EncodeToBytes(trx); return hex.EncodeToString(d) }(),
	)

	raw, _ := hex.DecodeString("f8db81a484aabbccdd20f840df947567d83b7b8d80addcb281a71d54fc7b3364ffed82271086000000606060df947567d83b7b8d80addcb281a71d54fc7b3364ffed824e20860000006060608180830334508083bc614ec101b882bad4d4401b1fb1c41d61727d7fd2aeb2bb3e65a27638a5326ca98404c0209ab159eaeb37f0ac75ed1ac44d92c3d17402d7d64b4c09664ae2698e1102448040c000f043fafeaf60343248a37e4f1d2743b4ab9116df6d627b4d8a874e4f48d3ae671c4e8d136eb87c544bea1763673a5f1762c2266364d1b22166d16e3872b5a9c700")
	var newTx *tx.Transaction
	if err := rlp.DecodeBytes(raw, &newTx); err != nil {
		t.Error(err)
	}
	assert.Equal(t, true, newTx.Features().IsDelegated())
	assert.Equal(t, "0x96c4cd08584994f337946f950eca5511abe15b152bc879bf47c2227901f9f2af", newTx.SigningHash().String())
	assert.Equal(t, "0xd989829d88b0ed1b06edf5c50174ecfa64f14a64", func() string { s, _ := newTx.Origin(); return s.String() }())
	assert.Equal(t, "0x956577b09b2a770d10ea129b26d916955df3606dc973da0043d6321b922fdef9", newTx.ID().String())
	assert.Equal(t, "0xd3ae78222beadb038203be21ed5ce7c9b1bff602", func() string { s, _ := newTx.Delegator(); return s.String() }())
}

func TestIntrinsicGas(t *testing.T) {
	gas, err := tx.IntrinsicGas()
	assert.Nil(t, err)
	assert.Equal(t, thor.TxGas+thor.ClauseGas, gas)

	gas, err = tx.IntrinsicGas(tx.NewClause(&thor.Address{}))
	assert.Nil(t, err)
	assert.Equal(t, thor.TxGas+thor.ClauseGas, gas)

	gas, err = tx.IntrinsicGas(tx.NewClause(nil))
	assert.Nil(t, err)
	assert.Equal(t, thor.TxGas+thor.ClauseGasContractCreation, gas)

	gas, err = tx.IntrinsicGas(tx.NewClause(&thor.Address{}), tx.NewClause(&thor.Address{}))
	assert.Nil(t, err)
	assert.Equal(t, thor.TxGas+thor.ClauseGas*2, gas)
}

func BenchmarkTxMining(b *testing.B) {
	tx := new(tx.Builder).Build()
	signer := thor.BytesToAddress([]byte("acc1"))
	maxWork := &big.Int{}
	eval := tx.EvaluateWork(signer)
	for i := 0; i < b.N; i++ {
		work := eval(uint64(i))
		if work.Cmp(maxWork) > 0 {
			maxWork = work
		}
	}
}

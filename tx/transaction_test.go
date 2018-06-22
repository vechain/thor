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

	k, _ := hex.DecodeString("7582be841ca040aa940fff6c05773129e135623e41acce3e0b8ba520dc1ae26a")
	priv, _ := crypto.ToECDSA(k)
	sig, _ := crypto.Sign(trx.SigningHash().Bytes(), priv)

	trx = trx.WithSignature(sig)
	assert.Equal(t, "0xd989829d88b0ed1b06edf5c50174ecfa64f14a64", func() string { s, _ := trx.Signer(); return s.String() }())
	assert.Equal(t, "0xda90eaea52980bc4bb8d40cb2ff84d78433b3b4a6e7d50b75736c5e3e77b71ec", trx.ID().String())

	assert.Equal(t, "f8970184aabbccdd20f840df947567d83b7b8d80addcb281a71d54fc7b3364ffed82271086000000606060df947567d83b7b8d80addcb281a71d54fc7b3364ffed824e208600000060606081808252088083bc614ec0b841f76f3c91a834165872aa9464fc55b03a13f46ea8d3b858e528fcceaf371ad6884193c3f313ff8effbb57fe4d1adc13dceb933bedbf9dbb528d2936203d5511df00",
		func() string { d, _ := rlp.EncodeToBytes(trx); return hex.EncodeToString(d) }(),
	)
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

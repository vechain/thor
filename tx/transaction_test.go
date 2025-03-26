// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
)

func GetMockTx(txType Type) *Transaction {
	to, _ := thor.ParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed")
	return NewBuilder(txType).ChainTag(1).
		BlockRef(BlockRef{0, 0, 0, 0, 0xaa, 0xbb, 0xcc, 0xdd}).
		Expiration(32).
		Clause(NewClause(&to).WithValue(big.NewInt(10000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
		Clause(NewClause(&to).WithValue(big.NewInt(20000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
		GasPriceCoef(128).
		MaxFeePerGas(big.NewInt(10000000)).
		MaxPriorityFeePerGas(big.NewInt(20000)).
		Gas(21000).
		DependsOn(nil).
		Nonce(12345678).Build()
}

func TestIsExpired(t *testing.T) {
	for _, txType := range []Type{TypeLegacy, TypeDynamicFee} {
		trx := GetMockTx(txType)
		res := trx.IsExpired(10)
		assert.Equal(t, res, false)
	}
}

func TestDependsOn(t *testing.T) {
	for _, txType := range []Type{TypeLegacy, TypeDynamicFee} {
		trx := GetMockTx(txType)
		res := trx.DependsOn()
		var expected *thor.Bytes32
		assert.Equal(t, expected, res)
	}
}

func TestTestFeatures(t *testing.T) {
	for _, txType := range []Type{TypeLegacy, TypeDynamicFee} {
		trx := GetMockTx(txType)
		supportedFeatures := trx.Features()
		res := trx.TestFeatures(supportedFeatures)
		assert.Equal(t, res, nil)
	}
}

func TestToString(t *testing.T) {
	test := []struct {
		name           string
		txType         Type
		expectedString string
	}{
		{
			name:           "Legacy transaction",
			txType:         TypeLegacy,
			expectedString: "\n\tTx(0x0000000000000000000000000000000000000000000000000000000000000000, 87 B)\n\tOrigin:         N/A\n\tClauses:        [\n\t\t(To:\t0x7567d83b7b8d80addcb281a71d54fc7b3364ffed\n\t\t Value:\t10000\n\t\t Data:\t0x000000606060) \n\t\t(To:\t0x7567d83b7b8d80addcb281a71d54fc7b3364ffed\n\t\t Value:\t20000\n\t\t Data:\t0x000000606060)]\n\tGas:            21000\n\tChainTag:       1\n\tBlockRef:       0-aabbccdd\n\tExpiration:     32\n\tDependsOn:      nil\n\tNonce:          12345678\n\tUnprovedWork:   0\n\tDelegator:      N/A\n\tSignature:      0x\n\n\t\tGasPriceCoef:   128\n\t\t",
		},
		{
			name:           "Dynamic fee transaction",
			txType:         TypeDynamicFee,
			expectedString: "\n\tTx(0x0000000000000000000000000000000000000000000000000000000000000000, 93 B)\n\tOrigin:         N/A\n\tClauses:        [\n\t\t(To:\t0x7567d83b7b8d80addcb281a71d54fc7b3364ffed\n\t\t Value:\t10000\n\t\t Data:\t0x000000606060) \n\t\t(To:\t0x7567d83b7b8d80addcb281a71d54fc7b3364ffed\n\t\t Value:\t20000\n\t\t Data:\t0x000000606060)]\n\tGas:            21000\n\tChainTag:       1\n\tBlockRef:       0-aabbccdd\n\tExpiration:     32\n\tDependsOn:      nil\n\tNonce:          12345678\n\tUnprovedWork:   0\n\tDelegator:      N/A\n\tSignature:      0x\n\n\t\tMaxFeePerGas:   10000000\n\t\tMaxPriorityFeePerGas: 20000\n\t\t",
		},
	}

	for _, tc := range test {
		t.Run(tc.name, func(t *testing.T) {
			trx := GetMockTx(tc.txType)
			res := trx.String()
			assert.Equal(t, tc.expectedString, res)
		})
	}
}

func TestTxSize(t *testing.T) {
	test := []struct {
		name         string
		txType       Type
		expectedSize thor.StorageSize
	}{
		{
			name:         "Legacy transaction",
			txType:       TypeLegacy,
			expectedSize: thor.StorageSize(87),
		},
		{
			name:         "Dynamic fee transaction",
			txType:       TypeDynamicFee,
			expectedSize: thor.StorageSize(93),
		},
	}

	for _, tc := range test {
		t.Run(tc.name, func(t *testing.T) {
			trx := GetMockTx(tc.txType)
			res := trx.Size()
			assert.Equal(t, tc.expectedSize, res)
		})
	}
}

func TestProvedWork(t *testing.T) {
	getBlockID := func(_ uint32) (thor.Bytes32, error) {
		return thor.Bytes32{}, nil
	}

	for _, txType := range []Type{TypeLegacy, TypeDynamicFee} {
		trx := GetMockTx(txType)
		headBlockNum := uint32(20)
		provedWork, err := trx.ProvedWork(headBlockNum, getBlockID)
		assert.NoError(t, err)
		assert.Equal(t, common.Big0, provedWork)
	}
}

func TestChainTag(t *testing.T) {
	for _, txType := range []Type{TypeLegacy, TypeDynamicFee} {
		trx := GetMockTx(txType)
		res := trx.ChainTag()
		assert.Equal(t, res, uint8(0x1))
	}
}

func TestNonce(t *testing.T) {
	for _, txType := range []Type{TypeLegacy, TypeDynamicFee} {
		trx := GetMockTx(txType)
		res := trx.Nonce()
		assert.Equal(t, res, uint64(0xbc614e))
	}
}

func TestOverallGasPrice(t *testing.T) {
	// Mock or create a Transaction with necessary fields initialized
	trx := GetMockTx(TypeLegacy)

	// Define test cases
	testCases := []struct {
		name           string
		baseGasPrice   *big.Int
		provedWork     *big.Int
		expectedOutput *big.Int
	}{
		{
			name:           "Case 1: No proved work",
			baseGasPrice:   big.NewInt(1000),
			provedWork:     big.NewInt(0),
			expectedOutput: big.NewInt(1501),
		},
		{
			name:           "Case 1: Negative proved work",
			baseGasPrice:   big.NewInt(1000),
			provedWork:     big.NewInt(-100),
			expectedOutput: big.NewInt(1501),
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call OverallGasPrice
			result := trx.OverallGasPrice(tc.baseGasPrice, tc.provedWork)

			// Check the value of the result
			if result.Cmp(tc.expectedOutput) != 0 {
				t.Errorf("%s: expected %v, got %v", tc.name, tc.expectedOutput, result)
			}
		})
	}
}

func TestEvaluateWork(t *testing.T) {
	tests := []struct {
		name         string
		txType       Type
		expectedFunc func(b *big.Int) bool
	}{
		{
			name:   "LegacyTxType",
			txType: TypeLegacy,
			expectedFunc: func(res *big.Int) bool {
				return res.Cmp(big.NewInt(0)) > 0
			},
		},
		{
			name:   "DynamicFeeTxType",
			txType: TypeDynamicFee,
			expectedFunc: func(res *big.Int) bool {
				return res.Cmp(common.Big0) == 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origin := thor.BytesToAddress([]byte("origin"))
			trx := GetMockTx(tt.txType)

			// Returns a function
			evaluate := trx.EvaluateWork(origin)

			// Test with a range of nonce values
			for nonce := uint64(0); nonce < 100; nonce++ {
				work := evaluate(nonce)

				// Basic Assertions
				assert.NotNil(t, work)
				assert.True(t, tt.expectedFunc(work), "Work does not match")
			}
		})
	}
}

func TestLegacyTx(t *testing.T) {
	to, _ := thor.ParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed")
	trx := NewBuilder(TypeLegacy).ChainTag(1).
		BlockRef(BlockRef{0, 0, 0, 0, 0xaa, 0xbb, 0xcc, 0xdd}).
		Expiration(32).
		Clause(NewClause(&to).WithValue(big.NewInt(10000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
		Clause(NewClause(&to).WithValue(big.NewInt(20000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
		GasPriceCoef(128).
		Gas(21000).
		DependsOn(nil).
		Nonce(12345678).Build()

	assert.Equal(t, "0x2a1c25ce0d66f45276a5f308b99bf410e2fc7d5b6ea37a49f2ab9f1da9446478", trx.SigningHash().String())
	assert.Equal(t, thor.Bytes32{}, trx.ID())

	assert.Equal(t, uint64(21000), func() uint64 { t := NewBuilder(TypeLegacy).Build(); g, _ := t.IntrinsicGas(); return g }())
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
		func() string { d, _ := trx.MarshalBinary(); return hex.EncodeToString(d) }(),
	)

	assert.Equal(t, "f8970184aabbccdd20f840df947567d83b7b8d80addcb281a71d54fc7b3364ffed82271086000000606060df947567d83b7b8d80addcb281a71d54fc7b3364ffed824e208600000060606081808252088083bc614ec0b841f76f3c91a834165872aa9464fc55b03a13f46ea8d3b858e528fcceaf371ad6884193c3f313ff8effbb57fe4d1adc13dceb933bedbf9dbb528d2936203d5511df00",
		func() string { d, _ := rlp.EncodeToBytes(trx); return hex.EncodeToString(d) }(),
	)
}

func TestDelegatedTx(t *testing.T) {
	to, _ := thor.ParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed")
	origin, _ := hex.DecodeString("7582be841ca040aa940fff6c05773129e135623e41acce3e0b8ba520dc1ae26a")
	delegator, _ := hex.DecodeString("321d6443bc6177273b5abf54210fe806d451d6b7973bccc2384ef78bbcd0bf51")

	var feat Features
	feat.SetDelegated(true)

	trx := NewBuilder(TypeLegacy).ChainTag(0xa4).
		BlockRef(BlockRef{0, 0, 0, 0, 0xaa, 0xbb, 0xcc, 0xdd}).
		Expiration(32).
		Clause(NewClause(&to).WithValue(big.NewInt(10000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
		Clause(NewClause(&to).WithValue(big.NewInt(20000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
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
		func() string { d, _ := trx.MarshalBinary(); return hex.EncodeToString(d) }(),
	)

	raw, _ := hex.DecodeString("f8db81a484aabbccdd20f840df947567d83b7b8d80addcb281a71d54fc7b3364ffed82271086000000606060df947567d83b7b8d80addcb281a71d54fc7b3364ffed824e20860000006060608180830334508083bc614ec101b882bad4d4401b1fb1c41d61727d7fd2aeb2bb3e65a27638a5326ca98404c0209ab159eaeb37f0ac75ed1ac44d92c3d17402d7d64b4c09664ae2698e1102448040c000f043fafeaf60343248a37e4f1d2743b4ab9116df6d627b4d8a874e4f48d3ae671c4e8d136eb87c544bea1763673a5f1762c2266364d1b22166d16e3872b5a9c700")
	newTx := new(Transaction)
	if err := newTx.UnmarshalBinary(raw); err != nil {
		t.Error(err)
	}
	assert.Equal(t, true, newTx.Features().IsDelegated())
	assert.Equal(t, "0x96c4cd08584994f337946f950eca5511abe15b152bc879bf47c2227901f9f2af", newTx.SigningHash().String())
	assert.Equal(t, "0xd989829d88b0ed1b06edf5c50174ecfa64f14a64", func() string { s, _ := newTx.Origin(); return s.String() }())
	assert.Equal(t, "0x956577b09b2a770d10ea129b26d916955df3606dc973da0043d6321b922fdef9", newTx.ID().String())
	assert.Equal(t, "0xd3ae78222beadb038203be21ed5ce7c9b1bff602", func() string { s, _ := newTx.Delegator(); return s.String() }())

	b, err := rlp.EncodeToBytes(newTx)
	assert.Nil(t, err)
	assert.Equal(t, raw, b)
}

func TestDynamicFeeEncode(t *testing.T) {
	to, _ := thor.ParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed")
	origin, _ := hex.DecodeString("7582be841ca040aa940fff6c05773129e135623e41acce3e0b8ba520dc1ae26a")

	trx := NewBuilder(TypeDynamicFee).ChainTag(0xa4).
		BlockRef(BlockRef{0, 0, 0, 0, 0xaa, 0xbb, 0xcc, 0xdd}).
		Expiration(32).
		Clause(NewClause(&to).WithValue(big.NewInt(10000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
		Clause(NewClause(&to).WithValue(big.NewInt(20000)).WithData([]byte{0, 0, 0, 0x60, 0x60, 0x60})).
		GasPriceCoef(128).
		Gas(210000).
		DependsOn(nil).
		Nonce(12345678).Build()

	p1, _ := crypto.ToECDSA(origin)
	sig, _ := crypto.Sign(trx.SigningHash().Bytes(), p1)

	trx = trx.WithSignature(sig)

	encoded, err := trx.MarshalBinary()
	assert.Nil(t, err)

	var expected bytes.Buffer
	expected.WriteByte(trx.Type())

	rlp.Encode(&expected, []any{
		trx.body.chainTag(),
		trx.body.blockRef(),
		trx.body.expiration(),
		trx.body.clauses(),
		trx.body.gas(),
		trx.body.maxFeePerGas(),
		trx.body.maxPriorityFeePerGas(),
		trx.body.dependsOn(),
		trx.body.nonce(),
		trx.body.reserved(),
		trx.body.signature(),
	})

	assert.Equal(t, expected.Bytes(), encoded)
}

func TestIntrinsicGas(t *testing.T) {
	gas, err := IntrinsicGas()
	assert.Nil(t, err)
	assert.Equal(t, thor.TxGas+thor.ClauseGas, gas)

	gas, err = IntrinsicGas(NewClause(&thor.Address{}))
	assert.Nil(t, err)
	assert.Equal(t, thor.TxGas+thor.ClauseGas, gas)

	gas, err = IntrinsicGas(NewClause(nil))
	assert.Nil(t, err)
	assert.Equal(t, thor.TxGas+thor.ClauseGasContractCreation, gas)

	gas, err = IntrinsicGas(NewClause(&thor.Address{}), NewClause(&thor.Address{}))
	assert.Nil(t, err)
	assert.Equal(t, thor.TxGas+thor.ClauseGas*2, gas)
}

func BenchmarkTxMining(b *testing.B) {
	for _, txType := range []Type{TypeLegacy, TypeDynamicFee} {
		trx := NewBuilder(txType).Build()
		signer := thor.BytesToAddress([]byte("acc1"))
		maxWork := &big.Int{}
		eval := trx.EvaluateWork(signer)
		for i := 0; i < b.N; i++ {
			work := eval(uint64(i))
			if work.Cmp(maxWork) > 0 {
				maxWork = work
			}
		}
	}
}

func FuzzTransactionMarshalling(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte, ui8 uint8, ui32 uint32, ui64 uint64) {
		txType := TypeLegacy
		if ui8%2 == 0 {
			txType = TypeDynamicFee
		}
		newTx := randomTx(b, ui8, ui32, ui64, txType)
		enc, err := newTx.MarshalBinary()
		if err != nil {
			t.Errorf("MarshalBinary: %v", err)
		}
		decTx := new(Transaction)
		err = decTx.UnmarshalBinary(enc)
		if err != nil {
			t.Errorf("UnmarshalBinary: %v", err)
		}
		if err := checkTxsEquality(newTx, decTx); err != nil {
			t.Errorf("Tx expected to be the same but: %v", err)
		}
	})
}

func randomTx(b []byte, ui8 uint8, ui32 uint32, ui64 uint64, txType Type) *Transaction {
	to := datagen.RandAddress()
	var b8 [8]byte
	copy(b8[:], b)
	i64 := int64(ui64)
	tag := datagen.RandBytes(1)[0]
	tr := NewBuilder(txType).ChainTag(tag).
		BlockRef(b8).
		Expiration(ui32).
		Clause(NewClause(&to).WithValue(big.NewInt(i64)).WithData(b)).
		Clause(NewClause(&to).WithValue(big.NewInt(i64)).WithData(b)).
		GasPriceCoef(ui8).
		MaxFeePerGas(big.NewInt(i64)).
		MaxPriorityFeePerGas(big.NewInt(i64)).
		Gas(ui64).
		DependsOn(nil).
		Nonce(ui64).Build()

	priv, _ := crypto.HexToECDSA("99f0500549792796c14fed62011a51081dc5b5e68fe8bd8a13b86be829c4fd36")
	sig, _ := crypto.Sign(tr.SigningHash().Bytes(), priv)

	tr = tr.WithSignature(sig)
	return tr
}

func checkTxsEquality(expectedTx, actualTx *Transaction) error {
	if expectedTx.ID() != actualTx.ID() {
		return fmt.Errorf("ID: expected %v, got %v", expectedTx.ID(), actualTx.ID())
	}
	if expectedTx.Hash() != actualTx.Hash() {
		return fmt.Errorf("Hash: expected %v, got %v", expectedTx.Hash(), actualTx.Hash())
	}
	if expectedTx.SigningHash() != actualTx.SigningHash() {
		return fmt.Errorf("SigningHash: expected %v, got %v", expectedTx.SigningHash(), actualTx.SigningHash())
	}
	return nil
}

// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/thor"
)

func GetMockTx(txType Type) *Transaction {
	to, _ := thor.ParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed")
	randTxID := thor.MustParseBytes32("0xd44cb3e0aa94b99331dc277895a004a8028c6a067deb207e18f0d7ac7cc13b30")
	var features Features
	features.SetDelegated(true)

	return NewBuilder(txType).ChainTag(1).
		BlockRef(BlockRef{0, 0, 0, 0, 0xaa, 0xbb, 0xcc, 0xdd}).
		Expiration(32).
		Clause(NewClause(&to).WithValue(big.NewInt(10000)).WithData([]byte{0, 0, 0, 0x61, 0x62, 0x63})).
		Clause(NewClause(&to).WithValue(big.NewInt(20000)).WithData([]byte{0, 0, 0, 0x66, 0x65, 0x64})).
		GasPriceCoef(128).
		MaxFeePerGas(big.NewInt(10000000)).
		MaxPriorityFeePerGas(big.NewInt(20000)).
		Gas(21000).
		DependsOn(&randTxID).
		Features(features).
		Nonce(12345678).Build()
}

func TestTransactionFields(t *testing.T) {
	// Define the transaction types to test
	txTypes := []Type{TypeLegacy, TypeDynamicFee}
	randTxID := thor.MustParseBytes32("0xd44cb3e0aa94b99331dc277895a004a8028c6a067deb207e18f0d7ac7cc13b30")

	for _, txType := range txTypes {
		t.Run(fmt.Sprintf("txType-%v", txType), func(t *testing.T) {
			trx := GetMockTx(txType)

			// Check that the transaction is not expired with a threshold of 10.
			assert.False(t, trx.IsExpired(10), "expected transaction of type %v to not be expired", txType)

			// Check that the DependsOn field is nil.
			assert.Equal(t, trx.DependsOn(), &randTxID, "expected DependsOn to be true for txType %v", txType)

			// Check that testing the features against itself yields no error.
			assert.NoError(t, trx.TestFeatures(trx.Features()), "expected TestFeatures to pass for txType %v", txType)

			// Verify the ChainTag is 0x1.
			assert.Equal(t, uint8(0x1), trx.ChainTag(), "expected ChainTag to be 0x1 for txType %v", txType)

			// Verify the Nonce is as expected (0xbc614e == 12345678 in decimal).
			assert.Equal(t, uint64(12345678), trx.Nonce(), "expected Nonce to be 12345678 for txType %v", txType)
		})
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
			expectedString: "\n\tTx(0x0000000000000000000000000000000000000000000000000000000000000000, 120 B)\n\tOrigin:         N/A\n\tClauses:        [\n\t\t(To:\t0x7567d83b7b8d80addcb281a71d54fc7b3364ffed\n\t\t Value:\t10000\n\t\t Data:\t0x000000616263) \n\t\t(To:\t0x7567d83b7b8d80addcb281a71d54fc7b3364ffed\n\t\t Value:\t20000\n\t\t Data:\t0x000000666564)]\n\tGas:            21000\n\tChainTag:       1\n\tBlockRef:       0-aabbccdd\n\tExpiration:     32\n\tDependsOn:      0xd44cb3e0aa94b99331dc277895a004a8028c6a067deb207e18f0d7ac7cc13b30\n\tNonce:          12345678\n\tUnprovedWork:   0\n\tDelegator:      N/A\n\tSignature:      0x\n\n\t\tGasPriceCoef:   128\n\t\t",
		},
		{
			name:           "Dynamic fee transaction",
			txType:         TypeDynamicFee,
			expectedString: "\n\tTx(0x0000000000000000000000000000000000000000000000000000000000000000, 126 B)\n\tOrigin:         N/A\n\tClauses:        [\n\t\t(To:\t0x7567d83b7b8d80addcb281a71d54fc7b3364ffed\n\t\t Value:\t10000\n\t\t Data:\t0x000000616263) \n\t\t(To:\t0x7567d83b7b8d80addcb281a71d54fc7b3364ffed\n\t\t Value:\t20000\n\t\t Data:\t0x000000666564)]\n\tGas:            21000\n\tChainTag:       1\n\tBlockRef:       0-aabbccdd\n\tExpiration:     32\n\tDependsOn:      0xd44cb3e0aa94b99331dc277895a004a8028c6a067deb207e18f0d7ac7cc13b30\n\tNonce:          12345678\n\tUnprovedWork:   0\n\tDelegator:      N/A\n\tSignature:      0x\n\n\t\tMaxFeePerGas:   10000000\n\t\tMaxPriorityFeePerGas: 20000\n\t\t",
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
			expectedSize: thor.StorageSize(120),
		},
		{
			name:         "Dynamic fee transaction",
			txType:       TypeDynamicFee,
			expectedSize: thor.StorageSize(126),
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

	// for dynamic fee, overallGasPrice should be maxFeePerGas
	trx = GetMockTx(TypeDynamicFee)
	assert.Equal(t, trx.OverallGasPrice(big.NewInt(1), big.NewInt(0)), trx.MaxFeePerGas())
}

func TestEvaluateWork(t *testing.T) {
	trx := GetMockTx(TypeLegacy)
	origin := thor.BytesToAddress([]byte("origin"))
	// Returns a function
	evaluate := trx.body.(*legacyTransaction).evaluateWork(origin)

	// Test with a range of nonce values
	for nonce := uint64(0); nonce < 100; nonce++ {
		work := evaluate(nonce)

		// Basic Assertions
		assert.NotNil(t, work)
		assert.True(t, work.Cmp(big.NewInt(0)) > 0, "legacy tx should have work")
	}

	trx = GetMockTx(TypeDynamicFee)
	assert.True(t, trx.UnprovedWork().Cmp(big.NewInt(0)) == 0, "dynamic fee tx should not have work")
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
		trx.body.maxPriorityFeePerGas(),
		trx.body.maxFeePerGas(),
		trx.body.gas(),
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
	trx := NewBuilder(TypeLegacy).Build()
	signer := thor.BytesToAddress([]byte("acc1"))
	maxWork := &big.Int{}
	eval := trx.body.(*legacyTransaction).evaluateWork(signer)
	for i := 0; i < b.N; i++ {
		work := eval(uint64(i))
		if work.Cmp(maxWork) > 0 {
			maxWork = work
		}
	}
}

func TestLegacyHashWithoutNonceFieldsIntegrity(t *testing.T) {
	mockTx := GetMockTx(TypeLegacy)

	origin, _ := mockTx.Origin()
	workHash := mockTx.body.(*legacyTransaction).hashWithoutNonce(origin)

	assert.Equal(t, workHash.String(), "0x9a5494c15870eb0981a09a1c35f5f34b6fcefe3b14c793797833ac7b1fb27864")
}

func TestHashingFieldsIntegrity(t *testing.T) {
	mockTx := GetMockTx(TypeLegacy)

	fieldsHash := thor.Blake2bFn(func(w io.Writer) {
		rlp.Encode(w, mockTx.body.(*legacyTransaction).signingFields())
	})

	assert.Equal(t, fieldsHash.String(), "0xce14ed5355e855725646507cf50579f4e3679280f6e4ac79a98bb7c69ae84a4a")

	mockTx = GetMockTx(TypeDynamicFee)

	fieldsHash = thor.Blake2bFn(func(w io.Writer) {
		rlp.Encode(w, mockTx.body.(*dynamicFeeTransaction).signingFields())
	})

	assert.Equal(t, fieldsHash.String(), "0x0b196cb741cc67de0bc389b9da472ebf9b6282ad226325a19fdb8c3d8941086e")
}

func TestTransactionHash(t *testing.T) {
	for range 100 {
		num := datagen.RandUint64()
		var builder *Builder
		if num%2 == 0 {
			builder = NewBuilder(TypeLegacy)
		} else {
			builder = NewBuilder(TypeDynamicFee)
		}

		var (
			ref  BlockRef
			feat Features
		)
		randBytes := datagen.RandBytes(9)
		copy(ref[:], randBytes[1:9])
		to := datagen.RandAddress()

		builder.
			ChainTag(randBytes[0]).
			BlockRef(ref).
			Expiration(uint32(num)).
			Clause(NewClause(&to).WithValue(datagen.RandBigInt()).WithData(datagen.RandBytes(32))).
			GasPriceCoef(uint8(num)).
			Gas(datagen.RandUint64()).
			Nonce(num)

		if num%3 == 0 {
			feat.SetDelegated(true)
		}
		builder.Features(feat)

		if num%5 == 0 {
			dep := datagen.RandomHash()
			builder.DependsOn(&dep)
		}

		trx := builder.Build()

		var expected thor.Bytes32
		if trx.Type() == TypeLegacy {
			expected = thor.Blake2bFn(func(w io.Writer) {
				err := rlp.Encode(w, signingFields(trx.body))
				assert.Nil(t, err)
			})

			// test evaluate work
			origin := thor.BytesToAddress(datagen.RandBytes(20))
			nonce := datagen.RandUint64()

			body, _ := trx.body.(*legacyTransaction)
			expectedWork := evaluateWork(body, origin, nonce, t)
			assert.Equal(t, expectedWork, trx.body.(*legacyTransaction).evaluateWork(origin)(nonce))
		} else {
			expected = thor.Blake2bFn(func(w io.Writer) {
				_, err := w.Write([]byte{trx.Type()})
				assert.Nil(t, err)
				err = rlp.Encode(w, signingFields(trx.body))
				assert.Nil(t, err)
			})
		}
		assert.Equal(t, len(trx.body.signingFields()), reflect.TypeOf(trx.body).Elem().NumField()-1, "unexpected number of signing fields")
		assert.Equal(t, expected, trx.SigningHash())
	}
}

// a reflect based implementation of hashWithoutNonce for cross implementation.
func evaluateWork(body *legacyTransaction, origin thor.Address, nonce uint64, t *testing.T) *big.Int {
	types := reflect.TypeOf(body)
	values := reflect.ValueOf(body)

	fields := make([]any, 0)
	// hash without nonce
	for i := range types.Elem().NumField() {
		// skip signature and nonce field
		if types.Elem().Field(i).Name != "Signature" && types.Elem().Field(i).Name != "Nonce" {
			// pass reserved field as a pointer
			if types.Elem().Field(i).Name == "Reserved" {
				reserved := values.Elem().Field(i).Interface().(reserved)
				fields = append(fields, &reserved)
			} else {
				fields = append(fields, values.Elem().Field(i).Interface())
			}
		}
	}
	fields = append(fields, origin)
	hashWithoutNonce := thor.Blake2bFn(func(w io.Writer) {
		err := rlp.Encode(w, fields)
		assert.Nil(t, err)
	})

	var nonceBytes [8]byte
	binary.BigEndian.PutUint64(nonceBytes[:], nonce)
	hash := thor.Blake2b(hashWithoutNonce[:], nonceBytes[:])
	r := new(big.Int).SetBytes(hash[:])
	return r.Div(math.MaxBig256, r)
}

// signingFields returns the fields that need to be signed.
// this is a reflect based implementation used for cross checking.
func signingFields(body txData) []any {
	types := reflect.TypeOf(body)
	values := reflect.ValueOf(body)

	fields := make([]any, 0)
	for i := range types.Elem().NumField() {
		// skip signature field
		if types.Elem().Field(i).Name != "Signature" {
			// pass reserved field as a pointer
			if types.Elem().Field(i).Name == "Reserved" {
				reserved := values.Elem().Field(i).Interface().(reserved)
				fields = append(fields, &reserved)
			} else {
				fields = append(fields, values.Elem().Field(i).Interface())
			}
		}
	}
	return fields
}

func TestTxFields(t *testing.T) {
	trx := GetMockTx(TypeDynamicFee)

	assert.Equal(t, trx.Expiration(), uint32(32))
	assert.Equal(t, trx.Gas(), uint64(21000))
	assert.Equal(t, trx.MaxFeePerGas(), big.NewInt(10000000))
	assert.Equal(t, trx.MaxPriorityFeePerGas(), big.NewInt(20000))
	assert.Equal(t, trx.Features(), Features(1))
	assert.Equal(t, trx.Nonce(), uint64(12345678))

	trx = NewBuilder(TypeLegacy).GasPriceCoef(128).Build()
	assert.Equal(t, trx.GasPriceCoef(), uint8(128))
}

func TestEffectiveGasPrice(t *testing.T) {
	bgp := big.NewInt(255)

	// for legacy tx, effective gas price is (1+gasPriceCoef/255)*baseGasPrice
	trx := NewBuilder(TypeLegacy).Build()
	assert.Equal(t, trx.EffectiveGasPrice(bgp, nil), big.NewInt(255))
	trx = NewBuilder(TypeLegacy).GasPriceCoef(255).Build()
	assert.Equal(t, trx.EffectiveGasPrice(bgp, nil), big.NewInt(255+255))
	trx = NewBuilder(TypeLegacy).GasPriceCoef(128).Build()
	assert.Equal(t, trx.EffectiveGasPrice(bgp, nil), big.NewInt(255+128))

	// for dynamic fee tx, effective takes baseFee into account
	b10 := big.NewInt(10)
	b5 := big.NewInt(5)
	b1 := big.NewInt(1)

	trx = NewBuilder(TypeDynamicFee).MaxFeePerGas(b10).MaxPriorityFeePerGas(b5).Build()
	// baseFee < maxFee - maxPriorityFee
	assert.Equal(t, trx.EffectiveGasPrice(nil, b1), big.NewInt(6))
	// baseFee = maxFee - maxPriorityFee
	trx = NewBuilder(TypeDynamicFee).MaxFeePerGas(b10).MaxPriorityFeePerGas(b5).Build()
	assert.Equal(t, trx.EffectiveGasPrice(nil, b5), big.NewInt(10))
	// baseFee > maxFee - maxPriorityFee
	trx = NewBuilder(TypeDynamicFee).MaxFeePerGas(b10).MaxPriorityFeePerGas(b5).Build()
	assert.Equal(t, trx.EffectiveGasPrice(nil, big.NewInt(6)), big.NewInt(10))
}

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
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

func GetMockTx() tx.Transaction {
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

	return *trx
}

func TestIsExpired(t *testing.T) {
	tx := GetMockTx()
	res := tx.IsExpired(10)
	assert.Equal(t, res, false)
}

func TestHash(t *testing.T) {
	tx := GetMockTx()
	res := tx.Hash()
	assert.Equal(t, res, thor.Bytes32{0x4b, 0xff, 0x70, 0x1, 0xfe, 0xc4, 0x2, 0x84, 0xd9, 0x3b, 0x4c, 0x45, 0x61, 0x7d, 0xc7, 0x41, 0xb9, 0xa8, 0x8e, 0xd5, 0x9d, 0xf, 0x1, 0xa3, 0x76, 0x39, 0x4c, 0x7b, 0xfe, 0xa6, 0xed, 0x24})
}

func TestDependsOn(t *testing.T) {
	tx := GetMockTx()
	res := tx.DependsOn()
	var expected *thor.Bytes32 = nil
	assert.Equal(t, expected, res)
}

func TestTestFeatures(t *testing.T) {
	txx := GetMockTx()
	supportedFeatures := tx.Features(1)
	res := txx.TestFeatures(supportedFeatures)
	assert.Equal(t, res, nil)
}

func TestToString(t *testing.T) {
	tx := GetMockTx() // Ensure this mock transaction has all the necessary fields populated

	// Construct the expected string representation of the transaction
	// This should match the format used in the String() method of the Transaction struct
	// and should reflect the actual state of the mock transaction
	expectedString := "\n\tTx(0x0000000000000000000000000000000000000000000000000000000000000000, 87 B)\n\tOrigin:         N/A\n\tClauses:        [\n\t\t(To:\t0x7567d83b7b8d80addcb281a71d54fc7b3364ffed\n\t\t Value:\t10000\n\t\t Data:\t0x000000606060) \n\t\t(To:\t0x7567d83b7b8d80addcb281a71d54fc7b3364ffed\n\t\t Value:\t20000\n\t\t Data:\t0x000000606060)]\n\tGasPriceCoef:   128\n\tGas:            21000\n\tChainTag:       1\n\tBlockRef:       0-aabbccdd\n\tExpiration:     32\n\tDependsOn:      nil\n\tNonce:          12345678\n\tUnprovedWork:   0\n\tDelegator:      N/A\n\tSignature:      0x\n"

	res := tx.String()

	// Use assert.Equal to compare the actual result with the expected string
	assert.Equal(t, expectedString, res)
}

func TestTxSize(t *testing.T) {
	tx := GetMockTx()

	size := tx.Size()
	assert.Equal(t, size, thor.StorageSize(87))
}

func TestProvedWork(t *testing.T) {
	// Mock the transaction
	tx := GetMockTx()

	// Define a head block number
	headBlockNum := uint32(20)

	// Mock getBlockID function
	getBlockID := func(num uint32) (thor.Bytes32, error) {
		return thor.Bytes32{}, nil
	}

	// Call ProvedWork
	provedWork, err := tx.ProvedWork(headBlockNum, getBlockID)

	// Check for errors
	assert.NoError(t, err)

	expectedProvedWork := big.NewInt(0)
	assert.Equal(t, expectedProvedWork, provedWork)
}

func TestChainTag(t *testing.T) {
	tx := GetMockTx()
	res := tx.ChainTag()
	assert.Equal(t, res, uint8(0x1))
}

func TestNonce(t *testing.T) {
	tx := GetMockTx()
	res := tx.Nonce()
	assert.Equal(t, res, uint64(0xbc614e))
}

func TestOverallGasPrice(t *testing.T) {
	// Mock or create a Transaction with necessary fields initialized
	tx := GetMockTx()

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
			result := tx.OverallGasPrice(tc.baseGasPrice, tc.provedWork)

			// Check the value of the result
			if result.Cmp(tc.expectedOutput) != 0 {
				t.Errorf("%s: expected %v, got %v", tc.name, tc.expectedOutput, result)
			}
		})
	}
}

func TestEvaluateWork(t *testing.T) {
	origin := thor.BytesToAddress([]byte("origin"))
	tx := GetMockTx()

	// Returns a function
	evaluate := tx.EvaluateWork(origin)

	// Test with a range of nonce values
	for nonce := uint64(0); nonce < 10; nonce++ {
		work := evaluate(nonce)

		// Basic Assertions
		assert.NotNil(t, work)
		assert.True(t, work.Cmp(big.NewInt(0)) > 0, "Work should be positive")
	}
}

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

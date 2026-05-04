// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
)

// Parse pipeline tests — exercise ParseEthTransaction and its validation steps.

func TestEth1559_ValidDecodeHashAndSender(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	trx, err := ParseEthTransaction(rawBytes, testChainID)
	require.NoError(t, err)

	assert.Equal(t, TypeEthTyped1559, trx.Type())
	sender, err := trx.Origin()
	require.NoError(t, err)
	assert.Equal(t, testSenderAddress, sender)
	assert.Equal(t, uint64(3), trx.Nonce())
	assert.Equal(t, uint64(21000), trx.Gas())
	assert.Equal(t, big.NewInt(10e9), trx.MaxFeePerGas())
	assert.Equal(t, big.NewInt(1e9), trx.MaxPriorityFeePerGas())
	assert.Equal(t, testChainID, trx.EthChainID())

	expectedHash := thor.Keccak256(rawBytes)
	assert.Equal(t, expectedHash, trx.ID())
	encoded, err := trx.MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, rawBytes, encoded)
}

func TestEth1559_ChainIDMismatch(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey) // signed for 1337
	require.NoError(t, err)

	_, err = ParseEthTransaction(rawBytes, 1)
	require.Error(t, err)
	assert.Equal(t, EthErrChainIDMismatch, err.(*EthTxError).Code)
}

func TestEth1559_MaxPriorityFeeExceedsMaxFeeRejected(t *testing.T) {
	// maxFeePerGas default is 10e9; set priority = maxFee + 1 so priority > max.
	rawBytes, err := defaultEth1559Builder().
		MaxPriorityFeePerGas(new(big.Int).Add(big.NewInt(10e9), big.NewInt(1))).
		BuildRaw(ethTestKey)
	require.NoError(t, err)

	_, err = ParseEthTransaction(rawBytes, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrFeeInconsistency, err.(*EthTxError).Code)
}

func TestEth1559_MaxPriorityFeeEqualMaxFeeAccepted(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().
		MaxPriorityFeePerGas(big.NewInt(10e9)). // equal to maxFeePerGas
		BuildRaw(ethTestKey)
	require.NoError(t, err)

	_, err = ParseEthTransaction(rawBytes, testChainID)
	require.NoError(t, err)
}

func TestEth1559_MaxPriorityFeeZeroAccepted(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().
		MaxPriorityFeePerGas(big.NewInt(0)). // zero tip is valid
		BuildRaw(ethTestKey)
	require.NoError(t, err)

	_, err = ParseEthTransaction(rawBytes, testChainID)
	require.NoError(t, err)
}

func TestEth1559_InvalidYParity(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	// Strip 0x02 prefix, decode, tamper yParity, re-encode.
	var body eth1559Transaction
	require.NoError(t, rlp.DecodeBytes(rawBytes[1:], &body))
	body.YParity = 2

	bodyRLP, err := rlp.EncodeToBytes(&body)
	require.NoError(t, err)
	tampered := append([]byte{TypeEthTyped1559}, bodyRLP...)

	_, err = ParseEthTransaction(tampered, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrInvalidField, err.(*EthTxError).Code)
}

func TestEth1559_NonCanonicalRLPRejected(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	var valid eth1559Transaction
	require.NoError(t, rlp.DecodeBytes(rawBytes[1:], &valid))

	type txBodyRaw struct {
		ChainID              *big.Int
		Nonce                uint64
		MaxPriorityFeePerGas rlp.RawValue // non-canonical injection
		MaxFeePerGas         *big.Int
		GasLimit             uint64
		To                   *thor.Address `rlp:"nil"`
		Value                *big.Int
		Data                 []byte
		AccessList           []AccessListEntry
		YParity              uint8
		R                    *big.Int
		S                    *big.Int
	}
	// Non-canonical encoding of maxPriorityFeePerGas: 2-byte string [0x00, 0x01]
	nonCanonical := []byte{0x82, 0x00, 0x01}
	bodyRLP, err := rlp.EncodeToBytes(&txBodyRaw{
		ChainID:              valid.ChainID,
		Nonce:                valid.Nonce,
		MaxPriorityFeePerGas: nonCanonical,
		MaxFeePerGas:         valid.MaxFeePerGas,
		GasLimit:             valid.GasLimit,
		To:                   valid.To,
		Value:                valid.Value,
		AccessList:           valid.AccessList,
		YParity:              valid.YParity,
		R:                    valid.R,
		S:                    valid.S,
	})
	require.NoError(t, err)
	tampered := append([]byte{TypeEthTyped1559}, bodyRLP...)

	_, err = ParseEthTransaction(tampered, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrNonCanonicalRLP, err.(*EthTxError).Code)
}

func TestEth1559_ContractCreation(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().To(nil).BuildRaw(ethTestKey)
	require.NoError(t, err)

	trx, err := ParseEthTransaction(rawBytes, testChainID)
	require.NoError(t, err)
	assert.Nil(t, trx.Clauses()[0].To())
}

func TestEth1559_NonEmptyAccessListRejected(t *testing.T) {
	to := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	storageKey := thor.MustParseBytes32("0x0000000000000000000000000000000000000000000000000000000000000001")

	// EthBuilder always encodes an empty access list. Build a correctly-signed
	// EIP-1559 tx with a non-empty access list directly using internal types so
	// that ParseEthTransaction can reach the access-list validation check.
	body := &eth1559Transaction{
		ChainID:              new(big.Int).SetUint64(testChainID),
		Nonce:                3,
		MaxPriorityFeePerGas: big.NewInt(1e9),
		MaxFeePerGas:         big.NewInt(10e9),
		GasLimit:             21000,
		To:                   &to,
		Value:                big.NewInt(1e9),
		AccessList:           []AccessListEntry{{Address: to, StorageKeys: []thor.Bytes32{storageKey}}},
	}
	sig, err := crypto.Sign(body.ethSigningHash().Bytes(), ethTestKey)
	require.NoError(t, err)
	body.YParity = sig[64]
	body.R = new(big.Int).SetBytes(sig[:32])
	body.S = new(big.Int).SetBytes(sig[32:64])

	bodyRLP, err := rlp.EncodeToBytes(body)
	require.NoError(t, err)
	rawBytes := append([]byte{TypeEthTyped1559}, bodyRLP...)

	_, err = ParseEthTransaction(rawBytes, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrAccessListUnsupported, err.(*EthTxError).Code)
}

func TestEth1559_EmptyAccessListAccepted(t *testing.T) {
	// EthBuilder always encodes an empty access list; verify that ParseEthTransaction
	// accepts the resulting wire bytes (wallets often send an explicit empty list).
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	_, err = ParseEthTransaction(rawBytes, testChainID)
	require.NoError(t, err)
}

func TestEth1559_HashIsStable(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	trx1, err := ParseEthTransaction(rawBytes, testChainID)
	require.NoError(t, err)
	trx2, err := ParseEthTransaction(rawBytes, testChainID)
	require.NoError(t, err)

	assert.Equal(t, trx1.ID(), trx2.ID(), "ethTxHash must be bit-for-bit stable across calls")
}

func TestEth1559_HashNotEqualSigningHash(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	trx, err := ParseEthTransaction(rawBytes, testChainID)
	require.NoError(t, err)

	var body eth1559Transaction
	require.NoError(t, rlp.DecodeBytes(rawBytes[1:], &body))
	signingHash := body.ethSigningHash()

	assert.NotEqual(t, trx.ID(), signingHash, "ethTxHash must differ from signing hash")
}

func TestEth1559_HighSRejected(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	var body eth1559Transaction
	require.NoError(t, rlp.DecodeBytes(rawBytes[1:], &body))
	body.S = new(big.Int).Set(secp256k1N)

	bodyRLP, err := rlp.EncodeToBytes(&body)
	require.NoError(t, err)
	tampered := append([]byte{TypeEthTyped1559}, bodyRLP...)

	_, err = ParseEthTransaction(tampered, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrHighSSignature, err.(*EthTxError).Code)
}

// Transaction object behaviour tests — properties of *Transaction returned by ParseEthTransaction.

// TestParseEthTransaction_1559 verifies observable properties of a parsed EIP-1559 transaction.
func TestParseEthTransaction_1559(t *testing.T) {
	to := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	trx, err := defaultEth1559Builder().Build(ethTestKey)
	require.NoError(t, err)

	assert.Equal(t, TypeEthTyped1559, trx.Type())

	origin, err := trx.Origin()
	require.NoError(t, err)
	assert.Equal(t, testSenderAddress, origin)

	assert.Equal(t, uint64(21000), trx.Gas())
	assert.Equal(t, big.NewInt(10e9), trx.MaxFeePerGas())
	assert.Equal(t, big.NewInt(1e9), trx.MaxPriorityFeePerGas())

	clauses := trx.Clauses()
	require.Len(t, clauses, 1)
	assert.Equal(t, &to, clauses[0].To())
	assert.Equal(t, big.NewInt(1e9), clauses[0].Value())
	assert.Empty(t, clauses[0].Data())

	// VeChain stubs: chain tag returns 0 (Ethereum txs use chainID for replay protection).
	assert.Equal(t, byte(0), trx.ChainTag())
	assert.Equal(t, uint32(0), trx.BlockRef().Number())
	assert.Equal(t, uint32(maxUint32), trx.Expiration())
	assert.False(t, trx.IsExpired(uint32(maxUint32)), "Ethereum txs must not expire at any uint32 block number")
	assert.Nil(t, trx.DependsOn())
	assert.Equal(t, Features(0), trx.Features(), "no VeChain feature flags on Ethereum txs")
}

// TestParseEthTransaction_1559ContractCreation verifies nil To (contract creation) for EIP-1559.
func TestParseEthTransaction_1559ContractCreation(t *testing.T) {
	trx, err := defaultEth1559Builder().To(nil).Build(ethTestKey)
	require.NoError(t, err)

	clauses := trx.Clauses()
	require.Len(t, clauses, 1)
	assert.True(t, clauses[0].IsCreatingContract())
}

// TestEth1559Tx_MarshalRoundtrip verifies that MarshalBinary followed by
// UnmarshalBinary produces a Transaction with identical observable properties.
func TestEth1559Tx_MarshalRoundtrip(t *testing.T) {
	testData := []byte{0xca, 0xfe, 0xba, 0xbe}
	original, err := defaultEth1559Builder().Data(testData).Build(ethTestKey)
	require.NoError(t, err)

	encoded, err := original.MarshalBinary()
	require.NoError(t, err)

	// Block-body encoding: first byte must be the 0x02 EIP-1559 type byte.
	require.NotEmpty(t, encoded)
	assert.Equal(t, TypeEthTyped1559, encoded[0])

	var decoded Transaction
	require.NoError(t, decoded.UnmarshalBinary(encoded))

	assert.Equal(t, TypeEthTyped1559, decoded.Type())
	assert.Equal(t, original.ID(), decoded.ID())

	originOrig, err := original.Origin()
	require.NoError(t, err)
	originDecoded, err := decoded.Origin()
	require.NoError(t, err)
	assert.Equal(t, originOrig, originDecoded)

	assert.Equal(t, original.MaxFeePerGas(), decoded.MaxFeePerGas())
	assert.Equal(t, original.MaxPriorityFeePerGas(), decoded.MaxPriorityFeePerGas())
	assert.Equal(t, original.Gas(), decoded.Gas())
}

// TestEth1559Tx_Hash verifies that Hash() == ID() == ethTxHash for Ethereum txs,
// preventing the latent bug where unexported fields cause rlp.Encode to produce an empty
// encoding and every Ethereum tx would share the same Hash().
func TestEth1559Tx_Hash(t *testing.T) {
	tx1559, err := defaultEth1559Builder().Build(ethTestKey)
	require.NoError(t, err)
	assert.Equal(t, tx1559.ID(), tx1559.Hash(), "Eth1559: Hash must equal ID (ethTxHash)")
}

// TestEth1559Tx_Copy verifies that copy() deep-copies an EIP-1559 tx body.
func TestEth1559Tx_Copy(t *testing.T) {
	original, err := defaultEth1559Builder().Build(ethTestKey)
	require.NoError(t, err)

	copied := &Transaction{body: original.body.copy()}

	assert.Equal(t, original.ID(), copied.ID())
	assert.Equal(t, original.Gas(), copied.Gas())
	assert.Equal(t, original.MaxFeePerGas(), copied.MaxFeePerGas())
	assert.Equal(t, original.MaxPriorityFeePerGas(), copied.MaxPriorityFeePerGas())

	// Mutation isolation: mutating the original body must not affect the copy.
	original.body.(*eth1559TxData).maxFee.SetInt64(0)
	assert.Equal(t, big.NewInt(10e9), copied.MaxFeePerGas(),
		"copied body must not share maxFee pointer with original")
}

// TestEth1559Tx_SetSignaturePanics verifies that calling WithSignature on an
// Ethereum tx panics with a clear message.
func TestEth1559Tx_SetSignaturePanics(t *testing.T) {
	tx1559, err := defaultEth1559Builder().Build(ethTestKey)
	require.NoError(t, err)
	assert.Panics(t, func() { tx1559.WithSignature(make([]byte, 65)) })
}

// TestEth1559Tx_EffectiveGasPrice verifies the EIP-1559 formula
// min(maxFeePerGas, maxPriorityFeePerGas + baseFee) for EthTyped1559.
// Default builder: maxFeePerGas=10 Gwei, maxPriorityFeePerGas=1 Gwei.
func TestEth1559Tx_EffectiveGasPrice(t *testing.T) {
	trx, err := defaultEth1559Builder().Build(ethTestKey)
	require.NoError(t, err)

	tests := []struct {
		name     string
		baseFee  *big.Int
		expected *big.Int
	}{
		// priority(1e9) + baseFee(2e9) = 3e9 < maxFee(10e9) → uncapped
		{"uncapped", big.NewInt(2e9), big.NewInt(3e9)},
		// priority(1e9) + baseFee(12e9) = 13e9 > maxFee(10e9) → fee-capped
		{"fee_capped", big.NewInt(12e9), big.NewInt(10e9)},
		// priority(1e9) + baseFee(9e9) = 10e9 == maxFee(10e9) → boundary (capped)
		{"boundary", big.NewInt(9e9), big.NewInt(10e9)},
		// priority(1e9) + baseFee(0) = 1e9 → zero base fee
		{"zero_basefee", big.NewInt(0), big.NewInt(1e9)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, trx.EffectiveGasPrice(tc.baseFee, nil))
		})
	}
}

// TestEth1559Tx_EffectivePriorityFeePerGas verifies
// min(maxFeePerGas − baseFee, maxPriorityFeePerGas) for EthTyped1559.
func TestEth1559Tx_EffectivePriorityFeePerGas(t *testing.T) {
	trx1559, err := defaultEth1559Builder().Build(ethTestKey)
	require.NoError(t, err)

	tests := []struct {
		name     string
		baseFee  *big.Int
		expected *big.Int
	}{
		// EthTyped1559: maxFee=10 Gwei, maxPriority=1 Gwei
		// min(10e9 − 2e9, 1e9) = 1e9 (priority-capped)
		{"1559_priority_capped", big.NewInt(2e9), big.NewInt(1e9)},
		// min(10e9 − 9e9, 1e9) = 1e9 (boundary, priority-capped)
		{"1559_boundary", big.NewInt(9e9), big.NewInt(1e9)},
		// min(10e9 − 10e9, 1e9) = 0 (maxFee consumed by baseFee)
		{"1559_zero_priority", big.NewInt(10e9), big.NewInt(0)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// legacyTxBaseGasPrice and provedWork are only used for TypeLegacy (VeChain native);
			// pass nil for both.
			// Use Cmp instead of Equal: big.Int subtraction producing zero yields
			// abs:{} (empty slice) rather than abs:nil, which trips reflect.DeepEqual.
			got := trx1559.EffectivePriorityFeePerGas(tc.baseFee, nil, nil)
			assert.Equal(t, 0, tc.expected.Cmp(got), "expected %s, got %s", tc.expected, got)
		})
	}
}

// TestTransaction_EthChainID verifies that EthChainID() returns the embedded chain ID
// for TypeEthTyped1559 and zero for all VeChain-native types.
func TestTransaction_EthChainID(t *testing.T) {
	eth1559, err := defaultEth1559Builder().ChainID(99999).Build(ethTestKey)
	require.NoError(t, err)
	assert.Equal(t, uint64(99999), eth1559.EthChainID(), "EthTyped1559 must return its embedded chain ID")

	legacy := NewBuilder(TypeLegacy).Build()
	assert.Equal(t, uint64(0), legacy.EthChainID(), "TypeLegacy must return 0")

	dynFee := NewBuilder(TypeDynamicFee).Build()
	assert.Equal(t, uint64(0), dynFee.EthChainID(), "TypeDynamicFee must return 0")
}

// maxUint32 mirrors math.MaxUint32 for use in assertions without an import.
const maxUint32 = 1<<32 - 1

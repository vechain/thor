// Copyright (c) 2026 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
)

// Decode round-trip and observable-property tests for EIP-1559 typed
// transactions. Validation that previously lived in ParseEthTransaction
// (chain ID, fee inconsistency, access list, low-S, yParity, non-canonical
// RLP) is now enforced at the layer it semantically belongs to:
//   - chain ID  → txpool.validateTxBasics, packer.Adopt, consensus.validateBlockBody
//   - fee       → runtime.ResolveTransaction
//   - access    → runtime.ResolveTransaction
//   - low-S     → tx.Transaction.EnforceSignatureLowS / Origin recovery
//   - yParity   → ECDSA recovery during Origin (invalid V → recovery error)
//   - canonical → go-ethereum rlp's strict canonical decoder
// These layers carry their own tests; the cases below focus on the body's
// own contract: round-trip, hash stability, copy isolation, fee/priority
// math, and chain-id accessor.

// TestEthDynamicFeeTx_DecodeRoundtrip verifies wire-bytes ↔ *Transaction
// round-trip via UnmarshalBinary preserves all observable fields.
func TestEthDynamicFeeTx_DecodeRoundtrip(t *testing.T) {
	rawBytes, err := defaultEthDynamicFeeBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	var trx Transaction
	require.NoError(t, trx.UnmarshalBinary(rawBytes))

	assert.Equal(t, TypeEthDynamicFee, trx.Type())
	sender, err := trx.Origin()
	require.NoError(t, err)
	assert.Equal(t, testSenderAddress, sender)
	assert.Equal(t, uint64(3), trx.Nonce())
	assert.Equal(t, uint64(21000), trx.Gas())
	assert.Equal(t, big.NewInt(10e9), trx.MaxFeePerGas())
	assert.Equal(t, big.NewInt(1e9), trx.MaxPriorityFeePerGas())
	assert.Equal(t, new(big.Int).SetUint64(testChainID), trx.ChainID())

	expectedHash := thor.Keccak256(rawBytes)
	assert.Equal(t, expectedHash, trx.ID())
	encoded, err := trx.MarshalBinary()
	require.NoError(t, err)
	assert.Equal(t, rawBytes, encoded)
}

// TestEthDynamicFeeTx_ContractCreationRoundtrip verifies nil To round-trips.
func TestEthDynamicFeeTx_ContractCreationRoundtrip(t *testing.T) {
	rawBytes, err := defaultEthDynamicFeeBuilder().To(nil).BuildRaw(ethTestKey)
	require.NoError(t, err)

	var trx Transaction
	require.NoError(t, trx.UnmarshalBinary(rawBytes))
	require.Len(t, trx.Clauses(), 1)
	assert.Nil(t, trx.Clauses()[0].To())
}

// TestEthDynamicFeeTx_ToWireEncoding cross-validates the wire encoding of the To
// field against the canonical EIP-1559 layout. We decode our raw wire bytes
// (minus the 0x02 type prefix) through a struct that uses the standard
// `rlp:"nil"` tag on To — the same shape go-ethereum's DynamicFeeTx exposes.
// If our encoding is interoperable with eth tooling, this struct must round-
// trip the To field exactly.
//
// This guards two RLP behaviours of our []any-based signingFields path:
//   - nil *thor.Address → RLP empty string (0x80), matching rlp:"nil"
//   - non-nil *thor.Address → 20-byte address with 0x94 prefix
func TestEthDynamicFeeTx_ToWireEncoding(t *testing.T) {
	// canonicalEthTxStruct mirrors go-ethereum's DynamicFeeTx wire layout:
	// 12 fields with rlp:"nil" on To. Round-tripping our bytes through this
	// struct proves wire compatibility with eth tooling.
	type canonicalEthTxStruct struct {
		ChainID              *big.Int
		Nonce                uint64
		MaxPriorityFeePerGas *big.Int
		MaxFeePerGas         *big.Int
		Gas                  uint64
		To                   *common.Address `rlp:"nil"`
		Value                *big.Int
		Data                 []byte
		AccessList           []AccessListEntry
		YParity              uint8
		R                    *big.Int
		S                    *big.Int
	}

	tests := []struct {
		name string
		to   *thor.Address // nil = contract creation
	}{
		{"contract_call", func() *thor.Address {
			a := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
			return &a
		}()},
		{"contract_creation_nil_to", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rawBytes, err := defaultEthDynamicFeeBuilder().To(tc.to).BuildRaw(ethTestKey)
			require.NoError(t, err)

			// Strip 0x02 type prefix; decode the rlpBody through the canonical struct.
			require.NotEmpty(t, rawBytes)
			require.Equal(t, byte(TypeEthDynamicFee), rawBytes[0])

			var canonical canonicalEthTxStruct
			require.NoError(t, rlp.DecodeBytes(rawBytes[1:], &canonical),
				"wire bytes must decode through go-ethereum-shaped struct")

			if tc.to == nil {
				assert.Nil(t, canonical.To,
					"nil To must encode as RLP empty string and decode back to nil")
			} else {
				require.NotNil(t, canonical.To,
					"non-nil To must survive round-trip through rlp:\"nil\" struct")
				assert.Equal(t, common.Address(*tc.to), *canonical.To,
					"To bytes must match exactly")
			}
		})
	}
}

// TestEthDynamicFeeTx_HashIsStable verifies that ID/Hash are bit-for-bit stable
// across decode runs.
func TestEthDynamicFeeTx_HashIsStable(t *testing.T) {
	rawBytes, err := defaultEthDynamicFeeBuilder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	var trx1, trx2 Transaction
	require.NoError(t, trx1.UnmarshalBinary(rawBytes))
	require.NoError(t, trx2.UnmarshalBinary(rawBytes))

	assert.Equal(t, trx1.ID(), trx2.ID(), "ethTxHash must be bit-for-bit stable across decodes")
}

// TestEthDynamicFeeTx_IDDiffersFromSigningHash verifies that the eth tx ID
// (Keccak256 of the full wire bytes including V/R/S) differs from the
// signing hash (Keccak256 of the unsigned 9-field preimage).
func TestEthDynamicFeeTx_IDDiffersFromSigningHash(t *testing.T) {
	trx, err := defaultEthDynamicFeeBuilder().Build(ethTestKey)
	require.NoError(t, err)
	assert.NotEqual(t, trx.ID(), trx.SigningHash(),
		"ethTxHash must differ from signing hash (signature changes the wire bytes)")
}

// TestEthDynamicFeeTx_Properties verifies observable properties of an EIP-1559 transaction.
func TestEthDynamicFeeTx_Properties(t *testing.T) {
	to := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	trx, err := defaultEthDynamicFeeBuilder().Build(ethTestKey)
	require.NoError(t, err)

	assert.Equal(t, TypeEthDynamicFee, trx.Type())

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

// TestEthDynamicFeeTx_ContractCreation verifies nil To (contract creation).
func TestEthDynamicFeeTx_ContractCreation(t *testing.T) {
	trx, err := defaultEthDynamicFeeBuilder().To(nil).Build(ethTestKey)
	require.NoError(t, err)

	clauses := trx.Clauses()
	require.Len(t, clauses, 1)
	assert.True(t, clauses[0].IsCreatingContract())
}

// TestEthDynamicFeeTx_MarshalRoundtrip verifies that MarshalBinary followed by
// UnmarshalBinary produces a Transaction with identical observable properties.
func TestEthDynamicFeeTx_MarshalRoundtrip(t *testing.T) {
	testData := []byte{0xca, 0xfe, 0xba, 0xbe}
	original, err := defaultEthDynamicFeeBuilder().Data(testData).Build(ethTestKey)
	require.NoError(t, err)

	encoded, err := original.MarshalBinary()
	require.NoError(t, err)

	// Block-body encoding: first byte must be the 0x02 EIP-1559 type byte.
	require.NotEmpty(t, encoded)
	assert.Equal(t, TypeEthDynamicFee, encoded[0])

	var decoded Transaction
	require.NoError(t, decoded.UnmarshalBinary(encoded))

	assert.Equal(t, TypeEthDynamicFee, decoded.Type())
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

// TestEthDynamicFeeTx_Hash verifies that Hash() == ID() == ethTxHash for Ethereum txs,
// preventing the latent bug where unexported fields cause rlp.Encode to produce an empty
// encoding and every Ethereum tx would share the same Hash().
func TestEthDynamicFeeTx_Hash(t *testing.T) {
	tx1559, err := defaultEthDynamicFeeBuilder().Build(ethTestKey)
	require.NoError(t, err)
	assert.Equal(t, tx1559.ID(), tx1559.Hash(), "Eth tx: Hash must equal ID (ethTxHash)")
}

// TestEthDynamicFeeTx_Copy verifies that copy() deep-copies an EIP-1559 tx body.
func TestEthDynamicFeeTx_Copy(t *testing.T) {
	original, err := defaultEthDynamicFeeBuilder().Build(ethTestKey)
	require.NoError(t, err)

	copied := &Transaction{body: original.body.copy()}

	assert.Equal(t, original.ID(), copied.ID())
	assert.Equal(t, original.Gas(), copied.Gas())
	assert.Equal(t, original.MaxFeePerGas(), copied.MaxFeePerGas())
	assert.Equal(t, original.MaxPriorityFeePerGas(), copied.MaxPriorityFeePerGas())

	// Mutation isolation: mutating the original body must not affect the copy.
	original.body.(*ethDynamicFeeTransaction).MaxFeePerGas.SetInt64(0)
	assert.Equal(t, big.NewInt(10e9), copied.MaxFeePerGas(),
		"copied body must not share maxFee pointer with original")
}

// TestEthDynamicFeeTx_WithSignatureRefreshesEthHash verifies that re-signing an
// eth tx via WithSignature produces a fresh body with refreshed ethHash, so
// ID()/Hash() observe the new wire bytes rather than the original tx's hash.
func TestEthDynamicFeeTx_WithSignatureRefreshesEthHash(t *testing.T) {
	tx1559, err := defaultEthDynamicFeeBuilder().Build(ethTestKey)
	require.NoError(t, err)
	originalID := tx1559.ID()

	// Replace signature with all-zeros (semantically nonsense but fine for hash check).
	resigned := tx1559.WithSignature(make([]byte, 65))
	assert.NotEqual(t, originalID, resigned.ID(),
		"WithSignature must refresh ethHash so ID changes when V/R/S change")
}

// TestEthDynamicFeeTx_EffectiveGasPrice verifies the EIP-1559 formula
// min(maxFeePerGas, maxPriorityFeePerGas + baseFee) for EthDynamicFee.
// Default builder: maxFeePerGas=10 Gwei, maxPriorityFeePerGas=1 Gwei.
func TestEthDynamicFeeTx_EffectiveGasPrice(t *testing.T) {
	trx, err := defaultEthDynamicFeeBuilder().Build(ethTestKey)
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

// TestEthDynamicFeeTx_EffectivePriorityFeePerGas verifies
// min(maxFeePerGas − baseFee, maxPriorityFeePerGas) for EthDynamicFee.
func TestEthDynamicFeeTx_EffectivePriorityFeePerGas(t *testing.T) {
	trx1559, err := defaultEthDynamicFeeBuilder().Build(ethTestKey)
	require.NoError(t, err)

	tests := []struct {
		name     string
		baseFee  *big.Int
		expected *big.Int
	}{
		// EthDynamicFee: maxFee=10 Gwei, maxPriority=1 Gwei
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

// TestTransaction_ChainID verifies that ChainID() returns the embedded chain ID
// for TypeEthDynamicFee and nil for all VeChain-native types.
func TestTransaction_ChainID(t *testing.T) {
	eth1559, err := defaultEthDynamicFeeBuilder().ChainID(99999).Build(ethTestKey)
	require.NoError(t, err)
	assert.Equal(t, new(big.Int).SetUint64(99999), eth1559.ChainID(), "EthDynamicFee must return its embedded chain ID")

	legacy := NewBuilder(TypeLegacy).Build()
	assert.Nil(t, legacy.ChainID(), "TypeLegacy must return nil")

	dynFee := NewBuilder(TypeDynamicFee).Build()
	assert.Nil(t, dynFee.ChainID(), "TypeDynamicFee must return nil")
}

// maxUint32 mirrors math.MaxUint32 for use in assertions without an import.
const maxUint32 = 1<<32 - 1

// TestEthDynamicFeeTx_RejectsHighS verifies that an eth tx with a non-canonical
// (high-S) signature fails Origin recovery. validateSignatureFormat enforces
// EIP-2 low-S as part of the signature shape check for 0x02 — without this,
// flipping S yields two valid signatures with different IDs but the same
// origin, splitting the chain index.
func TestEthDynamicFeeTx_RejectsHighS(t *testing.T) {
	signed, err := defaultEthDynamicFeeBuilder().Build(ethTestKey)
	require.NoError(t, err)

	// Origin recovery must succeed on the canonical signature.
	_, err = signed.Origin()
	require.NoError(t, err)

	// Flip S to N - S to produce a high-S signature with the same origin.
	body := signed.body.(*ethDynamicFeeTransaction)
	body.S = new(big.Int).Sub(secp256k1N, body.S)
	signed.cache.origin = atomic.Value{} // invalidate cache so validateSignatureFormat runs
	signed.cache.signingHash = atomic.Value{}

	_, err = signed.Origin()
	assert.ErrorIs(t, err, ErrHighSInSignature)
}

// TestEthDynamicFeeTx_DecodePreservesAccessList verifies that a non-empty
// access list round-trips through RLP — the rejection happens at resolve
// time (runtime.ResolveTransaction), not decode time, so wire bytes from
// eth wallets carrying an access list still parse with the field intact.
func TestEthDynamicFeeTx_DecodePreservesAccessList(t *testing.T) {
	trx, err := defaultEthDynamicFeeBuilder().Build(ethTestKey)
	require.NoError(t, err)

	// Inject an access list and re-sign so the wire bytes carry it.
	body := trx.body.(*ethDynamicFeeTransaction)
	body.AccessList = []AccessListEntry{
		{Address: thor.Address{0x01}, StorageKeys: []thor.Bytes32{{0x02}}},
	}
	signed := MustSign(trx, ethTestKey)

	raw, err := signed.MarshalBinary()
	require.NoError(t, err)

	var decoded Transaction
	require.NoError(t, decoded.UnmarshalBinary(raw))
	got := decoded.AccessList()
	require.Len(t, got, 1)
	assert.Equal(t, thor.Address{0x01}, got[0].Address)
	require.Len(t, got[0].StorageKeys, 1)
	assert.Equal(t, thor.Bytes32{0x02}, got[0].StorageKeys[0])
}

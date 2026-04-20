// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
)

// newEthDynamicFeeUnsigned returns a zero-signature 0x02 tx ready to be signed.
func newEthDynamicFeeUnsigned(t *testing.T, chainID *big.Int) *Transaction {
	t.Helper()
	to, err := thor.ParseAddress("0x7567d83b7b8d80addcb281a71d54fc7b3364ffed")
	require.NoError(t, err)
	return NewBuilder(TypeEthDynamicFee).
		ChainID(chainID).
		EthTo(&to).
		EthValue(big.NewInt(1_000_000)).
		EthData([]byte{0xAB, 0xCD}).
		MaxFeePerGas(big.NewInt(1_000_000_000_000)).
		MaxPriorityFeePerGas(big.NewInt(1_000_000_000)).
		Gas(21_000).
		Nonce(7).
		Build()
}

// TestEthDynamicFee_BuildFields checks that the Builder produces a tx with the
// expected field values visible through the Transaction API.
func TestEthDynamicFee_BuildFields(t *testing.T) {
	chainID := big.NewInt(100009)
	trx := newEthDynamicFeeUnsigned(t, chainID)

	assert.Equal(t, uint8(TypeEthDynamicFee), trx.Type())
	assert.Equal(t, chainID, trx.ChainID())
	assert.Equal(t, uint64(7), trx.Nonce())
	assert.Equal(t, uint64(21_000), trx.Gas())
	assert.Equal(t, big.NewInt(1_000_000_000_000), trx.MaxFeePerGas())
	assert.Equal(t, big.NewInt(1_000_000_000), trx.MaxPriorityFeePerGas())
	assert.False(t, trx.IsExpired(1_000_000), "0x02 tx must never appear expired")
	assert.Equal(t, byte(0), trx.ChainTag(), "0x02 tx has no ChainTag")
	assert.Equal(t, uint32(0), trx.Expiration(), "0x02 tx has no Expiration")
	assert.Nil(t, trx.DependsOn(), "0x02 tx has no dependsOn")
	// Single synthetic clause carries (to, value, data).
	cls := trx.Clauses()
	require.Len(t, cls, 1)
	assert.Equal(t, big.NewInt(1_000_000), cls[0].Value())
	assert.Equal(t, []byte{0xAB, 0xCD}, cls[0].Data())
}

// TestEthDynamicFee_SignAndRecover verifies that a signature produced by
// go-ethereum's crypto.Sign (V∈{0,1}) lets Origin() recover the right address.
func TestEthDynamicFee_SignAndRecover(t *testing.T) {
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)
	expected := thor.Address(crypto.PubkeyToAddress(pk.PublicKey))

	trx := newEthDynamicFeeUnsigned(t, big.NewInt(100009))
	signed, err := Sign(trx, pk)
	require.NoError(t, err)

	got, err := signed.Origin()
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

// TestEthDynamicFee_SigningHashIsKeccak asserts the signing hash is
// Keccak256(0x02 || RLP(signingFields)).
func TestEthDynamicFee_SigningHashIsKeccak(t *testing.T) {
	trx := newEthDynamicFeeUnsigned(t, big.NewInt(42))

	var buf bytes.Buffer
	buf.WriteByte(TypeEthDynamicFee)
	err := rlp.Encode(&buf, trx.body.signingFields())
	require.NoError(t, err)
	expected := thor.Keccak256(buf.Bytes())

	assert.Equal(t, expected, trx.SigningHash())
}

// TestEthDynamicFee_CanonicalTxID confirms the canonical tx id matches the
// Ethereum-style Keccak256(0x02 || RLP(signed tx)).
func TestEthDynamicFee_CanonicalTxID(t *testing.T) {
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)

	signed, err := Sign(newEthDynamicFeeUnsigned(t, big.NewInt(42)), pk)
	require.NoError(t, err)

	raw, err := signed.MarshalBinary()
	require.NoError(t, err)
	expected := thor.Keccak256(raw)

	assert.Equal(t, expected, signed.CanonicalTxID())
}

// TestEthDynamicFee_EncodeDecodeRoundTrip checks that MarshalBinary /
// UnmarshalBinary is a bit-stable round trip.
func TestEthDynamicFee_EncodeDecodeRoundTrip(t *testing.T) {
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)

	signed, err := Sign(newEthDynamicFeeUnsigned(t, big.NewInt(42)), pk)
	require.NoError(t, err)

	raw, err := signed.MarshalBinary()
	require.NoError(t, err)

	decoded := new(Transaction)
	require.NoError(t, decoded.UnmarshalBinary(raw))

	assert.Equal(t, uint8(TypeEthDynamicFee), decoded.Type())
	assert.Equal(t, signed.CanonicalTxID(), decoded.CanonicalTxID())
	// Re-encode and assert bit-exact match.
	rawAgain, err := decoded.MarshalBinary()
	require.NoError(t, err)
	assert.True(t, bytes.Equal(raw, rawAgain))

	origA, err := signed.Origin()
	require.NoError(t, err)
	origB, err := decoded.Origin()
	require.NoError(t, err)
	assert.Equal(t, origA, origB)
}

// TestEthDynamicFee_DecodePreservesAccessList verifies that a non-empty
// access list round-trips through RLP — the rejection happens at resolve
// time, not decode time, to keep hashes bit-exact with Ethereum wallets.
func TestEthDynamicFee_DecodePreservesAccessList(t *testing.T) {
	trx := newEthDynamicFeeUnsigned(t, big.NewInt(42))
	body := trx.body.(*ethDynamicFeeTransaction)
	body.AccessList = AccessList{
		{Address: thor.Address{0x01}, StorageKeys: []thor.Bytes32{{0x02}}},
	}

	pk, err := crypto.GenerateKey()
	require.NoError(t, err)
	signed, err := Sign(trx, pk)
	require.NoError(t, err)

	raw, err := signed.MarshalBinary()
	require.NoError(t, err)

	decoded := new(Transaction)
	require.NoError(t, decoded.UnmarshalBinary(raw))
	got := decoded.AccessList()
	require.Len(t, got, 1)
	assert.Equal(t, thor.Address{0x01}, got[0].Address)
	require.Len(t, got[0].StorageKeys, 1)
	assert.Equal(t, thor.Bytes32{0x02}, got[0].StorageKeys[0])
}

// TestEthDynamicFee_TestFeaturesIgnoresDelegation verifies that delegation
// features are simply not applicable to 0x02 (returns nil regardless of
// supported mask).
func TestEthDynamicFee_TestFeaturesIgnoresDelegation(t *testing.T) {
	trx := newEthDynamicFeeUnsigned(t, big.NewInt(42))
	var delegation Features
	delegation.SetDelegated(true)
	assert.NoError(t, trx.TestFeatures(delegation))
	assert.NoError(t, trx.TestFeatures(0))
}

// TestEthDynamicFee_SignatureLengthMustBe65 ensures the 0x02 envelope always
// carries exactly one signature.
func TestEthDynamicFee_SignatureLengthMustBe65(t *testing.T) {
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)
	signed, err := Sign(newEthDynamicFeeUnsigned(t, big.NewInt(42)), pk)
	require.NoError(t, err)

	// Mutate signature length → origin recovery must fail.
	bad := signed.WithSignature(append(signed.Signature(), 0x00))
	_, err = bad.Origin()
	assert.Error(t, err)
}

// TestEthDynamicFee_SignatureVIsParity asserts that the signature's V byte is
// the parity bit (0 or 1) as required by EIP-1559, not 27/28.
func TestEthDynamicFee_SignatureVIsParity(t *testing.T) {
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)
	signed, err := Sign(newEthDynamicFeeUnsigned(t, big.NewInt(42)), pk)
	require.NoError(t, err)

	sig := signed.Signature()
	require.Len(t, sig, 65)
	assert.True(t, sig[64] == 0 || sig[64] == 1, "V must be 0 or 1, got %d", sig[64])
}

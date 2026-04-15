// Copyright (c) 2026 The VeChainThor developers

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

func TestEth1559_ValidDecodeHashAndSender(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	ntx, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)

	assert.Equal(t, TypeEthTyped1559, ntx.TxType)
	assert.Equal(t, testSenderAddress, ntx.Sender)
	assert.Equal(t, uint64(3), ntx.Nonce)
	assert.Equal(t, uint64(21000), ntx.GasLimit)
	assert.Equal(t, big.NewInt(10e9), ntx.MaxFeePerGas)
	assert.Equal(t, big.NewInt(1e9), ntx.MaxPriorityFeePerGas)
	assert.Nil(t, ntx.GasPrice)
	assert.Equal(t, testChainID, ntx.ChainID)

	expectedHash := thor.Keccak256(rawBytes)
	assert.Equal(t, expectedHash, ntx.Hash)
	assert.Equal(t, rawBytes, ntx.Raw)
}

func TestEth1559_ChainIDMismatch(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey) // signed for 1337
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(rawBytes, 1)
	require.Error(t, err)
	assert.Equal(t, EthErrChainIDMismatch, err.(*EthTxError).Code)
}

func TestEth1559_MaxPriorityFeeExceedsMaxFeeRejected(t *testing.T) {
	// maxFeePerGas default is 10e9; set priority = maxFee + 1 so priority > max.
	rawBytes, err := defaultEth1559Builder().
		MaxPriorityFeePerGas(new(big.Int).Add(big.NewInt(10e9), big.NewInt(1))).
		BuildRaw(ethTestKey)
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(rawBytes, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrFeeInconsistency, err.(*EthTxError).Code)
}

func TestEth1559_MaxPriorityFeeEqualMaxFeeAccepted(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().
		MaxPriorityFeePerGas(big.NewInt(10e9)). // equal to maxFeePerGas
		BuildRaw(ethTestKey)
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)
}

func TestEth1559_MaxPriorityFeeZeroAccepted(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().
		MaxPriorityFeePerGas(big.NewInt(0)). // zero tip is valid
		BuildRaw(ethTestKey)
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(rawBytes, testChainID)
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

	_, err = NormalizeEthereumTx(tampered, testChainID)
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

	_, err = NormalizeEthereumTx(tampered, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrNonCanonicalRLP, err.(*EthTxError).Code)
}

func TestEth1559_ContractCreation(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().To(nil).BuildRaw(ethTestKey)
	require.NoError(t, err)

	ntx, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)
	assert.Nil(t, ntx.To)
}

func TestEth1559_NonEmptyAccessListRejected(t *testing.T) {
	to := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	storageKey := thor.MustParseBytes32("0x0000000000000000000000000000000000000000000000000000000000000001")

	// EthBuilder always encodes an empty access list. Build a correctly-signed
	// EIP-1559 tx with a non-empty access list directly using internal types so
	// that NormalizeEthereumTx can reach the access-list validation check.
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

	_, err = NormalizeEthereumTx(rawBytes, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrAccessListUnsupported, err.(*EthTxError).Code)
}

func TestEth1559_EmptyAccessListAccepted(t *testing.T) {
	// EthBuilder always encodes an empty access list; verify that NormalizeEthereumTx
	// accepts the resulting wire bytes (wallets often send an explicit empty list).
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	_, err = NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)
}

func TestEth1559_HashIsStable(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	ntx1, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)
	ntx2, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)

	assert.Equal(t, ntx1.Hash, ntx2.Hash, "ethTxHash must be bit-for-bit stable across calls")
}

func TestEth1559_HashNotEqualSigningHash(t *testing.T) {
	rawBytes, err := defaultEth1559Builder().BuildRaw(ethTestKey)
	require.NoError(t, err)

	ntx, err := NormalizeEthereumTx(rawBytes, testChainID)
	require.NoError(t, err)

	var body eth1559Transaction
	require.NoError(t, rlp.DecodeBytes(rawBytes[1:], &body))
	signingHash := body.ethSigningHash()

	assert.NotEqual(t, ntx.Hash, signingHash, "ethTxHash must differ from signing hash")
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

	_, err = NormalizeEthereumTx(tampered, testChainID)
	require.Error(t, err)
	assert.Equal(t, EthErrHighSSignature, err.(*EthTxError).Code)
}

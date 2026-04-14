// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

// eth_test_helpers_test.go — shared helpers for Ethereum transaction engine tests.
//
// All test transactions are constructed from a deterministic private key so that
// expected hash and sender values are stable across runs.

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/thor"
)

const testChainID = uint64(1337)

// testKey is a deterministic private key used across all engine tests.
var testKey = func() *ecdsa.PrivateKey {
	k, err := crypto.HexToECDSA("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	if err != nil {
		panic(err)
	}
	return k
}()

var testSenderAddress = thor.Address(crypto.PubkeyToAddress(testKey.PublicKey))

// buildEthLegacyRaw constructs and signs an EthLegacy transaction, returning rawEthBytes.
func buildEthLegacyRaw(t *testing.T, params ethLegacyParams) []byte {
	t.Helper()

	// signing preimage: RLP([nonce, gasPrice, gasLimit, to, value, data, chainID, 0, 0])
	type signingBody struct {
		Nonce    uint64
		GasPrice *big.Int
		GasLimit uint64
		To       *thor.Address `rlp:"nil"`
		Value    *big.Int
		Data     []byte
		ChainID  *big.Int
		Zero1    uint64
		Zero2    uint64
	}
	preimage, err := rlp.EncodeToBytes(&signingBody{
		Nonce:    params.Nonce,
		GasPrice: params.GasPrice,
		GasLimit: params.GasLimit,
		To:       params.To,
		Value:    params.Value,
		Data:     params.Data,
		ChainID:  new(big.Int).SetUint64(params.ChainID),
	})
	require.NoError(t, err)

	signingHash := thor.Keccak256(preimage)
	sig, err := crypto.Sign(signingHash[:], params.Key)
	require.NoError(t, err)

	yParity := uint64(sig[64])
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	v := new(big.Int).SetUint64(yParity + 2*params.ChainID + 35)

	type txBody struct {
		Nonce    uint64
		GasPrice *big.Int
		GasLimit uint64
		To       *thor.Address `rlp:"nil"`
		Value    *big.Int
		Data     []byte
		V        *big.Int
		R        *big.Int
		S        *big.Int
	}
	rawBytes, err := rlp.EncodeToBytes(&txBody{
		Nonce:    params.Nonce,
		GasPrice: params.GasPrice,
		GasLimit: params.GasLimit,
		To:       params.To,
		Value:    params.Value,
		Data:     params.Data,
		V:        v,
		R:        r,
		S:        s,
	})
	require.NoError(t, err)
	return rawBytes
}

type ethLegacyParams struct {
	Key      *ecdsa.PrivateKey
	ChainID  uint64
	Nonce    uint64
	GasPrice *big.Int
	GasLimit uint64
	To       *thor.Address
	Value    *big.Int
	Data     []byte
}

// defaultLegacyParams returns a valid set of EthLegacy params using testKey and testChainID.
func defaultLegacyParams() ethLegacyParams {
	to := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	return ethLegacyParams{
		Key:      testKey,
		ChainID:  testChainID,
		Nonce:    5,
		GasPrice: big.NewInt(20e9),
		GasLimit: 21000,
		To:       &to,
		Value:    big.NewInt(1e9),
		Data:     nil,
	}
}

// buildEth1559Raw constructs and signs an EthTyped1559 transaction, returning rawEthBytes.
func buildEth1559Raw(t *testing.T, params eth1559Params) []byte {
	t.Helper()

	type signingBody struct {
		ChainID              *big.Int
		Nonce                uint64
		MaxPriorityFeePerGas *big.Int
		MaxFeePerGas         *big.Int
		GasLimit             uint64
		To                   *thor.Address `rlp:"nil"`
		Value                *big.Int
		Data                 []byte
		AccessList           []AccessListEntry
	}
	preimageRLP, err := rlp.EncodeToBytes(&signingBody{
		ChainID:              new(big.Int).SetUint64(params.ChainID),
		Nonce:                params.Nonce,
		MaxPriorityFeePerGas: params.MaxPriorityFeePerGas,
		MaxFeePerGas:         params.MaxFeePerGas,
		GasLimit:             params.GasLimit,
		To:                   params.To,
		Value:                params.Value,
		Data:                 params.Data,
		AccessList:           params.AccessList,
	})
	require.NoError(t, err)

	// Signing hash: Keccak256(0x02 || RLP(9-field body))
	preimage := append([]byte{TypeEthTyped1559}, preimageRLP...)
	signingHash := thor.Keccak256(preimage)

	sig, err := crypto.Sign(signingHash[:], params.Key)
	require.NoError(t, err)

	yParity := sig[64]
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])

	type txBody struct {
		ChainID              *big.Int
		Nonce                uint64
		MaxPriorityFeePerGas *big.Int
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
	bodyRLP, err := rlp.EncodeToBytes(&txBody{
		ChainID:              new(big.Int).SetUint64(params.ChainID),
		Nonce:                params.Nonce,
		MaxPriorityFeePerGas: params.MaxPriorityFeePerGas,
		MaxFeePerGas:         params.MaxFeePerGas,
		GasLimit:             params.GasLimit,
		To:                   params.To,
		Value:                params.Value,
		Data:                 params.Data,
		AccessList:           params.AccessList,
		YParity:              yParity,
		R:                    r,
		S:                    s,
	})
	require.NoError(t, err)

	return append([]byte{TypeEthTyped1559}, bodyRLP...)
}

type eth1559Params struct {
	Key                  *ecdsa.PrivateKey
	ChainID              uint64
	Nonce                uint64
	MaxPriorityFeePerGas *big.Int
	MaxFeePerGas         *big.Int
	GasLimit             uint64
	To                   *thor.Address
	Value                *big.Int
	Data                 []byte
	AccessList           []AccessListEntry
}

func default1559Params() eth1559Params {
	to := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	return eth1559Params{
		Key:                  testKey,
		ChainID:              testChainID,
		Nonce:                3,
		MaxPriorityFeePerGas: big.NewInt(1e9),
		MaxFeePerGas:         big.NewInt(10e9),
		GasLimit:             21000,
		To:                   &to,
		Value:                big.NewInt(1e9),
		Data:                 nil,
	}
}

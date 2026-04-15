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

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/vechain/thor/v2/thor"
)

const testChainID = uint64(1337)

// ethTestKey is a deterministic private key used across all engine tests.
var ethTestKey = func() *ecdsa.PrivateKey {
	k, err := crypto.HexToECDSA("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	if err != nil {
		panic(err)
	}
	return k
}()

var testSenderAddress = thor.Address(crypto.PubkeyToAddress(ethTestKey.PublicKey))

// defaultEthLegacyBuilder returns a pre-configured EthBuilder for an EIP-155 legacy
// transaction using ethTestKey and testChainID.  Callers can chain additional setters to
// override individual fields before calling BuildRaw(ethTestKey) or Build(ethTestKey).
func defaultEthLegacyBuilder() *EthBuilder {
	to := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	return NewEthBuilder(TypeEthLegacy).
		ChainID(testChainID).
		Nonce(5).
		GasPrice(big.NewInt(20e9)).
		GasLimit(21000).
		To(&to).
		Value(big.NewInt(1e9))
}

// defaultEth1559Builder returns a pre-configured EthBuilder for an EIP-1559 typed
// transaction using ethTestKey and testChainID.  Callers can chain additional setters to
// override individual fields before calling BuildRaw(ethTestKey) or Build(ethTestKey).
func defaultEth1559Builder() *EthBuilder {
	to := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	return NewEthBuilder(TypeEthTyped1559).
		ChainID(testChainID).
		Nonce(3).
		MaxPriorityFeePerGas(big.NewInt(1e9)).
		MaxFeePerGas(big.NewInt(10e9)).
		GasLimit(21000).
		To(&to).
		Value(big.NewInt(1e9))
}

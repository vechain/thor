// Copyright (c) 2026 The VeChainThor developers
//
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

// defaultEthDynamicFeeBuilder returns a pre-configured *Builder for an EIP-1559 typed
// transaction using ethTestKey and testChainID. Tests chain `.With…()` helpers
// (or thor Builder setters) and call `.SignRaw()` / `.Sign()` for the wire
// bytes / *Transaction respectively.
type ethTxFactory struct {
	b *Builder
}

func defaultEthDynamicFeeBuilder() *ethTxFactory {
	to := thor.MustParseAddress("0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	return &ethTxFactory{b: NewBuilder(TypeEthDynamicFee).
		ChainID(testChainID).
		Nonce(3).
		MaxPriorityFeePerGas(big.NewInt(1e9)).
		MaxFeePerGas(big.NewInt(10e9)).
		Gas(21000).
		To(&to).
		Value(big.NewInt(1e9))}
}

// MaxPriorityFeePerGas overrides the priority fee on the underlying builder.
func (f *ethTxFactory) MaxPriorityFeePerGas(v *big.Int) *ethTxFactory {
	f.b.MaxPriorityFeePerGas(v)
	return f
}

// ChainID overrides the chain id on the underlying builder.
func (f *ethTxFactory) ChainID(id uint64) *ethTxFactory {
	f.b.ChainID(id)
	return f
}

// To overrides the recipient (nil for contract creation).
func (f *ethTxFactory) To(to *thor.Address) *ethTxFactory {
	f.b.To(to)
	return f
}

// Data overrides the call data.
func (f *ethTxFactory) Data(d []byte) *ethTxFactory {
	f.b.Data(d)
	return f
}

// BuildRaw signs and returns canonical wire bytes for the eth tx.
func (f *ethTxFactory) BuildRaw(key *ecdsa.PrivateKey) ([]byte, error) {
	signed, err := Sign(f.b.Build(), key)
	if err != nil {
		return nil, err
	}
	return signed.MarshalBinary()
}

// Build signs and returns the *Transaction.
func (f *ethTxFactory) Build(key *ecdsa.PrivateKey) (*Transaction, error) {
	return Sign(f.b.Build(), key)
}

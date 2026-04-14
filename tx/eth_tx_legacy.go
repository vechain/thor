// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

// ethLegacyTransaction is the decoded representation of an Ethereum Legacy (EIP-155) transaction.
//
// Wire format:
//
//	rawEthBytes = RLP([nonce, gasPrice, gasLimit, to, value, data, v, r, s])
//
// The TypeEthLegacy (0x52) byte is used only as a block-body marker and is never
// part of rawEthBytes or the ethTxHash computation.
type ethLegacyTransaction struct {
	Nonce    uint64
	GasPrice *big.Int
	GasLimit uint64
	To       *thor.Address `rlp:"nil"` // nil = contract creation
	Value    *big.Int
	Data     []byte
	V        *big.Int // EIP-155: V = yParity + 2*CHAIN_ID + 35
	R        *big.Int
	S        *big.Int
}

// ethLegacySigningBody is the 9-field unsigned preimage for the EIP-155 signing hash.
// V/R/S are replaced by [CHAIN_ID, 0, 0].
type ethLegacySigningBody struct {
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

// ethSigningHash computes the EIP-155 signing preimage hash:
//
//	Keccak256(RLP([nonce, gasPrice, gasLimit, to, value, data, CHAIN_ID, 0, 0]))
func (t *ethLegacyTransaction) ethSigningHash() thor.Bytes32 {
	return ethKeccakRlpHash(&ethLegacySigningBody{
		Nonce:    t.Nonce,
		GasPrice: t.GasPrice,
		GasLimit: t.GasLimit,
		To:       t.To,
		Value:    t.Value,
		Data:     t.Data,
		ChainID:  t.chainID(),
	})
}

// TODO review this
// signature returns the normalised 65-byte ECDSA signature [R(32) || S(32) || yParity(1)].
func (t *ethLegacyTransaction) signature() []byte {
	sig := make([]byte, 65)
	t.R.FillBytes(sig[0:32])
	t.S.FillBytes(sig[32:64])
	sig[64] = t.yParity()
	return sig
}

// chainID extracts CHAIN_ID from V: chainID = (V − 35) / 2.
// Callers must ensure V ≥ 35 (enforced by the engine).
func (t *ethLegacyTransaction) chainID() *big.Int {
	return new(big.Int).Rsh(new(big.Int).Sub(t.V, big.NewInt(35)), 1)
}

// yParity extracts the recovery bit from V: yParity = (V − 35) & 1.
func (t *ethLegacyTransaction) yParity() byte {
	return byte(new(big.Int).Sub(t.V, big.NewInt(35)).Uint64() & 1)
}

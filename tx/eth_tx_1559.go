// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

// AccessListEntry is a single entry in an EIP-2930 / EIP-1559 access list.
// Non-empty access lists are rejected by the engine (EthErrAccessListUnsupported)
// until EIP-2930 warm/cold gas accounting is implemented.
type AccessListEntry struct {
	Address     thor.Address
	StorageKeys []thor.Bytes32
}

// eth1559Transaction is the decoded representation of an EIP-1559 typed transaction.
//
// Wire format:
//
//	rawEthBytes = 0x02 || RLP([chainId, nonce, maxPriorityFeePerGas, maxFeePerGas,
//	                            gasLimit, to, value, data, accessList,
//	                            yParity, r, s])
//
// The 0x02 type byte IS part of rawEthBytes and the ethTxHash computation.
type eth1559Transaction struct {
	ChainID              *big.Int
	Nonce                uint64
	MaxPriorityFeePerGas *big.Int
	MaxFeePerGas         *big.Int
	GasLimit             uint64
	To                   *thor.Address `rlp:"nil"` // nil = contract creation
	Value                *big.Int
	Data                 []byte
	AccessList           []AccessListEntry
	YParity              uint8
	R                    *big.Int
	S                    *big.Int
}

// eth1559SigningBody is the 9-field unsigned preimage for the EIP-1559 signing hash.
// YParity, R, S are omitted.
type eth1559SigningBody struct {
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

// ethSigningHash computes the EIP-1559 signing preimage hash:
//
//	Keccak256(0x02 || RLP([chainId, nonce, maxPriorityFeePerGas, maxFeePerGas,
//	                        gasLimit, to, value, data, accessList]))
func (t *eth1559Transaction) ethSigningHash() thor.Bytes32 {
	return ethKeccakPrefixedRlpHash(TypeEthTyped1559, &eth1559SigningBody{
		ChainID:              t.ChainID,
		Nonce:                t.Nonce,
		MaxPriorityFeePerGas: t.MaxPriorityFeePerGas,
		MaxFeePerGas:         t.MaxFeePerGas,
		GasLimit:             t.GasLimit,
		To:                   t.To,
		Value:                t.Value,
		Data:                 t.Data,
		AccessList:           t.AccessList,
	})
}

// signature returns the normalised 65-byte ECDSA signature [R(32) || S(32) || yParity(1)].
func (t *eth1559Transaction) signature() []byte {
	sig := make([]byte, 65)
	t.R.FillBytes(sig[0:32])
	t.S.FillBytes(sig[32:64])
	sig[64] = t.YParity
	return sig
}

// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"bytes"
	"io"
	"sync"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/vechain/thor/v2/thor"
)

// ethKeccakRlpHash computes Keccak256(RLP(x)). Used for Ethereum transaction hashing.
func ethKeccakRlpHash(x any) thor.Bytes32 {
	return thor.EthKeccak256Fn(func(w io.Writer) {
		rlp.Encode(w, x)
	})
}

// ethKeccakPrefixedRlpHash computes Keccak256(prefix || RLP(x)).
// Used for EIP-2718 typed transaction hashing (e.g., 0x02 prefix for EIP-1559).
func ethKeccakPrefixedRlpHash(prefix byte, x any) thor.Bytes32 {
	return thor.EthKeccak256Fn(func(w io.Writer) {
		w.Write([]byte{prefix})
		rlp.Encode(w, x)
	})
}

// deriveBufferPool holds temporary encoder buffers for DeriveSha and TX encoding.
var encodeBufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

func rlpHash(x any) thor.Bytes32 {
	return thor.Blake2bFn(func(w io.Writer) {
		rlp.Encode(w, x)
	})
}

// prefixedRlpHash writes the prefix into the hasher before rlp-encoding the
// given interface. It's used for typed transactions.
func prefixedRlpHash(prefix byte, x any) thor.Bytes32 {
	return thor.Blake2bFn(func(w io.Writer) {
		w.Write([]byte{prefix})
		rlp.Encode(w, x)
	})
}

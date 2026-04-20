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

// keccakPrefixedRlpHash writes the prefix byte followed by the RLP encoding of x
// into a Keccak256 hasher. Used for ETH EIP-1559 (0x02) typed transactions where
// the canonical hash must match Ethereum's keccak256(type || RLP(payload)).
func keccakPrefixedRlpHash(prefix byte, x any) thor.Bytes32 {
	buf := encodeBufferPool.Get().(*bytes.Buffer)
	defer encodeBufferPool.Put(buf)
	buf.Reset()
	buf.WriteByte(prefix)
	_ = rlp.Encode(buf, x)
	return thor.Keccak256(buf.Bytes())
}

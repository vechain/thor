// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"io"

	"github.com/vechain/thor/lowrlp"
)

// fastNodeEncoder is the fast node encoder using low-level rlp encoder.
type fastNodeEncoder struct{}

var frlp fastNodeEncoder

// Encode writes the RLP encoding of node to w.
func (fastNodeEncoder) Encode(w io.Writer, node node, nonCrypto bool) error {
	enc := lowrlp.NewEncoder()
	defer enc.Release()

	fastEncodeNode(enc, node, nonCrypto)
	return enc.ToWriter(w)
}

// EncodeToBytes returns the RLP encoding of node.
func (fastNodeEncoder) EncodeToBytes(collapsed node, nonCrypto bool) []byte {
	enc := lowrlp.NewEncoder()
	defer enc.Release()

	fastEncodeNode(enc, collapsed, nonCrypto)
	return enc.ToBytes()
}

func (fastNodeEncoder) EncodeTrailing(w io.Writer, collapsed node) error {
	enc := lowrlp.NewEncoder()
	defer enc.Release()
	fastEncodeNodeTrailing(enc, collapsed)
	return enc.ToWriter(w)
}

func fastEncodeNode(w *lowrlp.Encoder, collapsed node, nonCrypto bool) {
	switch n := collapsed.(type) {
	case *fullNode:
		offset := w.List()
		for _, c := range n.Children {
			if c != nil {
				fastEncodeNode(w, c, nonCrypto)
			} else {
				w.EncodeEmptyString()
			}
		}
		w.ListEnd(offset)
	case *shortNode:
		offset := w.List()
		w.EncodeString(n.Key)
		if n.Val != nil {
			fastEncodeNode(w, n.Val, nonCrypto)
		} else {
			w.EncodeEmptyString()
		}
		w.ListEnd(offset)
	case *hashNode:
		if nonCrypto {
			w.EncodeString(nonCryptoNodeHashPlaceholder)
		} else {
			w.EncodeString(n.Hash[:])
		}
	case *valueNode:
		w.EncodeString(n.Value)
	}
}

func fastEncodeNodeTrailing(w *lowrlp.Encoder, collapsed node) {
	switch n := collapsed.(type) {
	case *shortNode:
		fastEncodeNodeTrailing(w, n.Val)
	case *fullNode:
		for _, c := range n.Children {
			fastEncodeNodeTrailing(w, c)
		}
	case *valueNode:
		// skip empty node
		if len(n.Value) > 0 {
			w.EncodeString(n.meta)
		}
	case *hashNode:
		w.EncodeUint(n.seq)
	}
}

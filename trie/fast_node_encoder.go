// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"github.com/vechain/thor/lowrlp"
)

// implements node.encode and node.encodeTrailing

func (n *fullNode) encode(e *lowrlp.Encoder, nonCrypto bool) {
	off := e.List()
	for _, c := range n.Children {
		if c != nil {
			c.encode(e, nonCrypto)
		} else {
			e.EncodeEmptyString()
		}
	}
	e.ListEnd(off)
}

func (n *fullNode) encodeTrailing(e *lowrlp.Encoder) {
	for _, c := range n.Children {
		if c != nil {
			c.encodeTrailing(e)
		}
	}
}

func (n *shortNode) encode(e *lowrlp.Encoder, nonCrypto bool) {
	off := e.List()
	e.EncodeString(n.Key)
	if n.Val != nil {
		n.Val.encode(e, nonCrypto)
	} else {
		e.EncodeEmptyString()
	}
	e.ListEnd(off)
}

func (n *shortNode) encodeTrailing(e *lowrlp.Encoder) {
	if n.Val != nil {
		n.Val.encodeTrailing(e)
	}
}

func (n *hashNode) encode(e *lowrlp.Encoder, nonCrypto bool) {
	if nonCrypto {
		e.EncodeString(nonCryptoNodeHashPlaceholder)
	} else {
		e.EncodeString(n.Hash[:])
	}
}

func (n *hashNode) encodeTrailing(e *lowrlp.Encoder) {
	e.EncodeUint(n.seq)
}

func (n *valueNode) encode(e *lowrlp.Encoder, nonCrypto bool) {
	e.EncodeString(n.Value)
}

func (n *valueNode) encodeTrailing(e *lowrlp.Encoder) {
	if len(n.Value) > 0 {
		e.EncodeString(n.meta)
	}
}

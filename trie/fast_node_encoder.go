// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

import (
	"fmt"
	"io"

	"github.com/vechain/thor/lowrlp"
)

// fastNodeEncoder is the fast node encoder using low-level rlp encoder.
type fastNodeEncoder struct{}

var frlp fastNodeEncoder

// Encode writes the RLP encoding of node to w.
func (fastNodeEncoder) Encode(w io.Writer, node node) error {
	enc := lowrlp.NewEncoder()
	defer enc.Release()

	if err := fastEncodeNode(enc, node); err != nil {
		return err
	}
	return enc.ToWriter(w)
}

// EncodeToBytes returns the RLP encoding of node.
func (fastNodeEncoder) EncodeToBytes(node node) ([]byte, error) {
	enc := lowrlp.NewEncoder()
	defer enc.Release()

	if err := fastEncodeNode(enc, node); err != nil {
		return nil, err
	}
	return enc.ToBytes(), nil
}

func (fastNodeEncoder) EncodeTrailing(w io.Writer, node node) error {
	enc := lowrlp.NewEncoder()
	defer enc.Release()

	if err := fastEncodeNodeTrailing(enc, node); err != nil {
		return err
	}
	return enc.ToWriter(w)
}

func fastEncodeNode(w *lowrlp.Encoder, n node) error {
	switch n := n.(type) {
	case *fullNode:
		offset := w.List()
		for _, c := range n.Children {
			if c != nil {
				if err := fastEncodeNode(w, c); err != nil {
					return err
				}
			} else {
				w.EncodeEmptyString()
			}
		}
		w.ListEnd(offset)
	case *shortNode:
		offset := w.List()
		w.EncodeString(n.Key)
		if n.Val != nil {
			if err := fastEncodeNode(w, n.Val); err != nil {
				return err
			}
		} else {
			w.EncodeEmptyString()
		}
		w.ListEnd(offset)
	case *hashNode:
		w.EncodeString(n.Hash[:])
	case *valueNode:
		w.EncodeString(n.Value)
	default:
		return fmt.Errorf("unsupported node type: %T", n)
	}
	return nil
}

func fastEncodeNodeTrailing(w *lowrlp.Encoder, collapsed node) error {
	{
		var collectMeta func(n node)
		collectMeta = func(n node) {
			switch n := n.(type) {
			case *shortNode:
				collectMeta(n.Val)
			case *fullNode:
				for _, c := range n.Children {
					collectMeta(c)
				}
			case *valueNode:
				// skip empty node
				if len(n.Value) > 0 {
					w.EncodeString(n.meta)
				}
			}
		}

		listoffset := w.List()
		collectMeta(collapsed)
		w.ListEnd(listoffset)
	}

	{
		switch n := collapsed.(type) {
		case *shortNode:
			listoffset := w.List()
			if h, ok := n.Val.(*hashNode); ok {
				w.EncodeUint(uint64(h.cNum) | (uint64(h.dNum) << 32))
			}
			w.ListEnd(listoffset)
		case *fullNode:
			listoffset := w.List()
			for i := 0; i < 16; i++ {
				if h, ok := n.Children[i].(*hashNode); ok {
					w.EncodeUint(uint64(h.cNum) | (uint64(h.dNum) << 32))
				}
			}
			w.ListEnd(listoffset)
		default:
			return fmt.Errorf("encode trailing, unexpected node: %v", n)
		}
	}
	return nil
}

// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package trie

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/qianbin/drlp"
)

var indices = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e", "f", "[17]"}

// node kinds (lower 3 bits of node tag)
const (
	kindEmpty byte = iota
	kindFull
	kindShort
	kindRef
	kindValue
)

// note attributes (higher 5 bits of node tag)
const (
	attrHasHash    = byte(1 << iota) // indicates a ref node has the hash field
	attrHasMajor                     // indicates a ref node has the ver.Major field
	attrHasMinor                     // indicates a ref node has the ver.Minor field
	attrHasMeta                      // indicates a value node has the meta field
	attrHasManyRef                   // indicates a full node contains many ref nodes
)

type node interface {
	Version() Version
	fstring(string) string
	cache() (ref refNode, gen uint16, dirty bool)
	encodeConsensus(buf []byte) []byte // encode the node for computing MPT root
	encode(buf []byte, skipHash bool) []byte
}

type (
	fullNode struct {
		children [17]node
		flags    nodeFlag
	}
	shortNode struct {
		key   []byte
		child node
		flags nodeFlag
	}
	refNode struct {
		hash []byte
		ver  Version
	}
	valueNode struct {
		val  []byte
		meta []byte // metadata of the value
	}
)

func (n *fullNode) Version() Version  { return n.flags.ref.ver }
func (n *shortNode) Version() Version { return n.flags.ref.ver }
func (n *refNode) Version() Version   { return n.ver }
func (n *valueNode) Version() Version { return Version{} }

func (n *fullNode) copy() *fullNode   { copy := *n; return &copy }
func (n *shortNode) copy() *shortNode { copy := *n; return &copy }

// nodeFlag contains caching-related metadata about a node.
type nodeFlag struct {
	ref   refNode // cached ref of the node
	gen   uint16  // cache generation counter
	dirty bool    // whether the node has changes that must be written to the database
}

func (n *fullNode) cache() (refNode, uint16, bool)  { return n.flags.ref, n.flags.gen, n.flags.dirty }
func (n *shortNode) cache() (refNode, uint16, bool) { return n.flags.ref, n.flags.gen, n.flags.dirty }
func (n *refNode) cache() (refNode, uint16, bool)   { return *n, 0, false }
func (n *valueNode) cache() (refNode, uint16, bool) { return refNode{}, 0, true }

// Pretty printing.
func (n *fullNode) String() string  { return n.fstring("") }
func (n *shortNode) String() string { return n.fstring("") }
func (n *refNode) String() string   { return n.fstring("") }
func (n *valueNode) String() string { return n.fstring("") }

func (n *fullNode) fstring(ind string) string {
	resp := fmt.Sprintf("[\n%s  ", ind)
	for i, node := range n.children {
		if node == nil {
			resp += fmt.Sprintf("%s: <nil> ", indices[i])
		} else {
			resp += fmt.Sprintf("%s: %v", indices[i], node.fstring(ind+"  "))
		}
	}
	return resp + fmt.Sprintf("\n%s] ", ind)
}

func (n *shortNode) fstring(ind string) string {
	return fmt.Sprintf("{%x: %v} ", n.key, n.child.fstring(ind+"  "))
}

func (n *refNode) fstring(ind string) string {
	return fmt.Sprintf("<%x> #%v", n.hash, n.ver)
}

func (n *valueNode) fstring(ind string) string {
	return fmt.Sprintf("%x - %x", n.val, n.meta)
}

func mustDecodeNode(ref *refNode, buf []byte, cacheGen uint16) node {
	n, _, err := decodeNode(ref, buf, cacheGen)
	if err != nil {
		panic(fmt.Sprintf("node %v: %v", ref, err))
	}
	return n
}

// decodeNode parses a trie node in storage.
func decodeNode(ref *refNode, buf []byte, cacheGen uint16) (node, []byte, error) {
	if len(buf) == 0 {
		return nil, nil, io.ErrUnexpectedEOF
	}
	tag := buf[0]
	buf = buf[1:]
	kind, attrs := tag&0x7, tag>>3
	switch kind {
	case kindEmpty:
		return nil, buf, nil
	case kindFull:
		n, rest, err := decodeFull(ref, buf, cacheGen, attrs)
		if err != nil {
			return nil, nil, wrapError(err, "full")
		}
		return n, rest, nil
	case kindShort:
		n, rest, err := decodeShort(ref, buf, cacheGen, attrs)
		if err != nil {
			return nil, nil, wrapError(err, "short")
		}
		return n, rest, nil
	case kindRef:
		n, rest, err := decodeRef(&refNode{}, buf, attrs)
		if err != nil {
			return nil, nil, wrapError(err, "ref")
		}
		return n, rest, nil
	case kindValue:
		n, rest, err := decodeValue(buf, attrs)
		if err != nil {
			return nil, nil, wrapError(err, "value")
		}
		return n, rest, nil
	default:
		return nil, nil, fmt.Errorf("invalid node kind %v", kind)
	}
}

func decodeFull(ref *refNode, buf []byte, cacheGen uint16, attrs byte) (*fullNode, []byte, error) {
	var (
		n    = fullNode{flags: nodeFlag{gen: cacheGen}}
		err  error
		refs []refNode // prealloced ref nodes
	)
	if ref != nil {
		n.flags.ref = *ref
	} else {
		n.flags.dirty = true
	}

	// prealloc an array of refNode, to reduce alloc count
	if (attrs & attrHasManyRef) != 0 {
		refs = make([]refNode, 16)
	}

	for i := range n.children {
		if tag := buf[0]; tag&0x7 == kindRef {
			var ref *refNode
			if len(refs) > 0 {
				ref = &refs[0]
				refs = refs[1:]
			} else {
				ref = &refNode{}
			}
			if n.children[i], buf, err = decodeRef(ref, buf[1:], tag>>3); err != nil {
				return nil, nil, wrapError(err, fmt.Sprintf("[%d]", i))
			}
		} else {
			if n.children[i], buf, err = decodeNode(nil, buf, cacheGen); err != nil {
				return nil, nil, wrapError(err, fmt.Sprintf("[%d]", i))
			}
		}
	}
	return &n, buf, nil
}

func decodeShort(ref *refNode, buf []byte, cacheGen uint16, attrs byte) (*shortNode, []byte, error) {
	var (
		n          = shortNode{flags: nodeFlag{gen: cacheGen}}
		err        error
		compactKey []byte
	)
	if ref != nil {
		n.flags.ref = *ref
	} else {
		n.flags.dirty = true
	}

	// decode key
	if compactKey, buf, err = vp.SplitString(buf); err != nil {
		return nil, nil, err
	}
	n.key = compactToHex(compactKey)

	if hasTerm(n.key) {
		// decode value
		n.child, buf, err = decodeValue(buf, attrs)
	} else {
		// decode child node
		n.child, buf, err = decodeNode(nil, buf, cacheGen)
	}
	if err != nil {
		return nil, nil, err
	}
	return &n, buf, nil
}

func decodeValue(buf []byte, attrs byte) (*valueNode, []byte, error) {
	var (
		n   valueNode
		err error
	)
	// decode val
	if n.val, buf, err = vp.SplitString(buf); err != nil {
		return nil, nil, err
	}

	// decode meta
	if (attrs & attrHasMeta) != 0 {
		if n.meta, buf, err = vp.SplitString(buf); err != nil {
			return nil, nil, err
		}
	}
	return &n, buf, nil
}

func decodeRef(n *refNode, buf []byte, attrs byte) (*refNode, []byte, error) {
	// decode hash
	if (attrs & attrHasHash) != 0 {
		n.hash, buf = buf[:32], buf[32:]
	}

	// decode version
	if (attrs & attrHasMajor) != 0 {
		n.ver.Major, buf = binary.BigEndian.Uint32(buf), buf[4:]
	}
	if (attrs & attrHasMinor) != 0 {
		n.ver.Minor, buf = binary.BigEndian.Uint32(buf), buf[4:]
	}
	return n, buf, nil
}

// wraps a decoding error with information about the path to the
// invalid child node (for debugging encoding issues).
type decodeError struct {
	what  error
	stack []string
}

func wrapError(err error, ctx string) error {
	if err == nil {
		return nil
	}
	if decErr, ok := err.(*decodeError); ok {
		decErr.stack = append(decErr.stack, ctx)
		return decErr
	}
	return &decodeError{err, []string{ctx}}
}

func (err *decodeError) Error() string {
	return fmt.Sprintf("%v (decode path: %s)", err.what, strings.Join(err.stack, "<-"))
}

func (n *fullNode) encode(buf []byte, skipHash bool) []byte {
	var (
		tagPos   = len(buf)
		nRefNode = 0
	)
	// encode tag
	buf = append(buf, kindFull)

	// encode children
	for _, cn := range n.children {
		switch cn := cn.(type) {
		case *refNode:
			buf = cn.encode(buf, skipHash)
			nRefNode++
		case nil:
			buf = append(buf, kindEmpty)
		default:
			if ref, _, dirty := cn.cache(); dirty {
				buf = cn.encode(buf, skipHash)
			} else {
				buf = ref.encode(buf, skipHash)
			}
		}
	}
	if nRefNode > 4 {
		buf[tagPos] |= (attrHasManyRef << 3)
	}
	return buf
}

func (n *shortNode) encode(buf []byte, skipHash bool) []byte {
	var (
		attrs  byte
		tagPos = len(buf)
	)
	// encode tag
	buf = append(buf, kindShort)

	// encode key
	buf = vp.AppendUint32(buf, uint32(compactLen(n.key)))
	buf = appendHexToCompact(buf, n.key)

	if hasTerm(n.key) {
		vn := n.child.(*valueNode)
		// encode value
		buf = vp.AppendString(buf, vn.val)
		// encode meta
		if len(vn.meta) > 0 {
			attrs |= attrHasMeta
			buf = vp.AppendString(buf, vn.meta)
		}
		buf[tagPos] |= (attrs << 3)
	} else {
		// encode child node
		if ref, _, dirty := n.child.cache(); dirty {
			buf = n.child.encode(buf, skipHash)
		} else {
			buf = ref.encode(buf, skipHash)
		}
	}
	return buf
}

func (n *valueNode) encode(buf []byte, skipHash bool) []byte {
	var (
		attrs  byte
		tagPos = len(buf)
	)
	// encode tag
	buf = append(buf, kindValue)

	// encode value
	buf = vp.AppendString(buf, n.val)

	// encode meta
	if len(n.meta) > 0 {
		attrs |= attrHasMeta
		buf = vp.AppendString(buf, n.meta)
	}
	buf[tagPos] |= (attrs << 3)
	return buf
}

func (n *refNode) encode(buf []byte, skipHash bool) []byte {
	var (
		attrs  byte
		tagPos = len(buf)
	)
	// encode tag
	buf = append(buf, kindRef)
	// encode hash
	if !skipHash {
		attrs |= attrHasHash
		buf = append(buf, n.hash...)
	}
	// encode version
	if n.ver.Major != 0 {
		attrs |= attrHasMajor
		buf = binary.BigEndian.AppendUint32(buf, n.ver.Major)
	}
	if n.ver.Minor != 0 {
		attrs |= attrHasMinor
		buf = binary.BigEndian.AppendUint32(buf, n.ver.Minor)
	}
	buf[tagPos] |= (attrs << 3)
	return buf
}

//// encodeConsensus

func (n *fullNode) encodeConsensus(buf []byte) []byte {
	offset := len(buf)

	for _, cn := range n.children {
		switch cn := cn.(type) {
		case *refNode:
			buf = cn.encodeConsensus(buf)
		case nil:
			buf = drlp.AppendString(buf, nil)
		default:
			if ref, _, _ := cn.cache(); ref.hash != nil {
				buf = drlp.AppendString(buf, ref.hash)
			} else {
				buf = cn.encodeConsensus(buf)
			}
		}
	}
	return drlp.EndList(buf, offset)
}

func (n *shortNode) encodeConsensus(buf []byte) []byte {
	offset := len(buf)

	const maxHeaderSize = 5
	// reserve space for rlp string header
	buf = append(buf, make([]byte, maxHeaderSize)...)
	// compact the key just after reserved space
	buf = appendHexToCompact(buf, n.key)
	// encode the compact key in the right place
	buf = drlp.AppendString(buf[:offset], buf[offset+maxHeaderSize:])

	if ref, _, _ := n.child.cache(); ref.hash != nil {
		buf = drlp.AppendString(buf, ref.hash)
	} else {
		buf = n.child.encodeConsensus(buf)
	}

	return drlp.EndList(buf, offset)
}

func (n *valueNode) encodeConsensus(buf []byte) []byte {
	return drlp.AppendString(buf, n.val)
}

func (n *refNode) encodeConsensus(buf []byte) []byte {
	return drlp.AppendString(buf, n.hash)
}

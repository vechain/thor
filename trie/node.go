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
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

var indices = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e", "f", "[17]"}

type node interface {
	fstring(string) string
	cache() (*hashNode, bool)
	commitNum() uint32
	distinctNum() uint32
}

type (
	fullNode struct {
		Children [17]node // Actual trie node data to encode/decode (needs custom encoder)
		flags    nodeFlag
	}
	shortNode struct {
		Key   []byte
		Val   node
		flags nodeFlag
	}
	hashNode struct {
		Hash []byte
		cNum uint32 // the commit number
		dNum uint32 // the number to distinguish commits with the same commit number
	}
	valueNode struct {
		Value []byte
		meta  []byte // metadata of the value
	}
)

// EncodeRLP encodes a full node into the consensus RLP format.
func (n *fullNode) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, n.Children)
}

// EncodeRLP encodes a hash node into the consensus RLP format.
func (n *hashNode) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, n.Hash)
}

// EncodeRLP encodes a value node into the consensus RLP format.
func (n *valueNode) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, n.Value)
}

func (n *fullNode) copy() *fullNode   { copy := *n; return &copy }
func (n *shortNode) copy() *shortNode { copy := *n; return &copy }

// nodeFlag contains caching-related metadata about a node.
type nodeFlag struct {
	hash  *hashNode // cached hash of the node (may be nil)
	dirty bool      // whether the node has changes that must be written to the database
}

func (n *fullNode) cache() (*hashNode, bool)  { return n.flags.hash, n.flags.dirty }
func (n *shortNode) cache() (*hashNode, bool) { return n.flags.hash, n.flags.dirty }
func (n *hashNode) cache() (*hashNode, bool)  { return nil, true }
func (n *valueNode) cache() (*hashNode, bool) { return nil, true }

func (n *fullNode) commitNum() uint32 {
	if n.flags.hash != nil {
		return n.flags.hash.cNum
	}
	return 0
}

func (n *fullNode) distinctNum() uint32 {
	if n.flags.hash != nil {
		return n.flags.hash.dNum
	}
	return 0
}

func (n *shortNode) commitNum() uint32 {
	if n.flags.hash != nil {
		return n.flags.hash.cNum
	}
	return 0
}

func (n *shortNode) distinctNum() uint32 {
	if n.flags.hash != nil {
		return n.flags.hash.dNum
	}
	return 0
}

func (n *hashNode) commitNum() uint32   { return n.cNum }
func (n *hashNode) distinctNum() uint32 { return n.dNum }

func (n *valueNode) commitNum() uint32   { return 0 }
func (n *valueNode) distinctNum() uint32 { return 0 }

// Pretty printing.
func (n *fullNode) String() string  { return n.fstring("") }
func (n *shortNode) String() string { return n.fstring("") }
func (n *hashNode) String() string  { return n.fstring("") }
func (n *valueNode) String() string { return n.fstring("") }

func (n *fullNode) fstring(ind string) string {
	resp := fmt.Sprintf("[\n%s  ", ind)
	for i, node := range n.Children {
		if node == nil {
			resp += fmt.Sprintf("%s: <nil> ", indices[i])
		} else {
			resp += fmt.Sprintf("%s: %v", indices[i], node.fstring(ind+"  "))
		}
	}
	return resp + fmt.Sprintf("\n%s] ", ind)
}
func (n *shortNode) fstring(ind string) string {
	return fmt.Sprintf("{%x: %v} ", n.Key, n.Val.fstring(ind+"  "))
}
func (n *hashNode) fstring(ind string) string {
	return fmt.Sprintf("<%x> ", n.Hash)
}
func (n *valueNode) fstring(ind string) string {
	return fmt.Sprintf("%x ", n.Value)
}

// metaList is the splitted rlp list of metadata.
type metaList []byte

// Next returns the current metadata and move to the next one.
// It will return io.EOF when positioned on the end.
func (ml *metaList) Next() ([]byte, error) {
	if ml == nil {
		return nil, nil
	}
	if len(*ml) == 0 {
		return nil, io.EOF
	}

	content, rest, err := rlp.SplitString(*ml)
	if err != nil {
		return nil, err
	}

	*ml = rest
	return content, nil
}

// numList is a list of commit & distinct numbers.
type numList []byte

// Next returns the commit number of the current child hash node and move to the next none.
// It returns io.EOF if reaches end.
func (nl *numList) Next() (cNum uint32, dNum uint32, err error) {
	if nl == nil {
		return 0, 0, nil
	}
	if len(*nl) == 0 {
		return 0, 0, io.EOF
	}

	content, rest, err := rlp.SplitString(*nl)
	if err != nil {
		return 0, 0, err
	}

	if len(content) > 8 {
		return 0, 0, errors.New("encoded number too long")
	}

	*nl = rest

	var n uint64
	for _, b := range content {
		n <<= 8
		n |= uint64(b)
	}
	return uint32(n), uint32(n >> 32), nil
}

// decodeTrailing decodes the trailing buffer.
func decodeTrailing(buf []byte) (*metaList, *numList, error) {
	if len(buf) == 0 {
		return nil, nil, nil
	}

	mBuf, rest, err := rlp.SplitList(buf)
	if err != nil {
		return nil, nil, err
	}

	nBuf, rest, err := rlp.SplitList(rest)
	if err != nil {
		return nil, nil, err
	}
	if len(rest) > 0 {
		return nil, nil, errors.New("unexpected content after trailing")
	}

	return (*metaList)(&mBuf), (*numList)(&nBuf), nil
}

func mustDecodeNode(hash *hashNode, buf []byte) node {
	_, _, trailing, err := rlp.Split(buf)
	if err != nil {
		panic(fmt.Sprintf("node %x: %v", hash, err))
	}
	ml, cnl, err := decodeTrailing(trailing)
	if err != nil {
		panic(fmt.Sprintf("decode trailing, node %x: %v", hash, err))
	}
	buf = buf[:len(buf)-len(trailing)]
	n, err := decodeNode(hash, buf, ml, cnl)
	if err != nil {
		panic(fmt.Sprintf("node %x: %v", hash, err))
	}
	if (ml != nil && len(*ml) != 0) || (cnl != nil && len(*cnl) != 0) {
		panic(fmt.Sprintf("node %x: trailing buffer not fully consumed", hash))
	}
	return n
}

// decodeNode parses the RLP encoding of a trie node.
func decodeNode(hash *hashNode, buf []byte, ml *metaList, nl *numList) (node, error) {
	if len(buf) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	elems, _, err := rlp.SplitList(buf)
	if err != nil {
		return nil, fmt.Errorf("decode error: %v", err)
	}
	switch c, _ := rlp.CountValues(elems); c {
	case 2:
		n, err := decodeShort(hash, buf, elems, ml, nl)
		return n, wrapError(err, "short")
	case 17:
		n, err := decodeFull(hash, buf, elems, ml, nl)
		return n, wrapError(err, "full")
	default:
		return nil, fmt.Errorf("invalid number of list elements: %v", c)
	}
}

func decodeShort(hash *hashNode, buf, elems []byte, ml *metaList, nl *numList) (*shortNode, error) {
	kbuf, rest, err := rlp.SplitString(elems)
	if err != nil {
		return nil, err
	}
	flag := nodeFlag{hash: hash}
	key := compactToHex(kbuf)
	if hasTerm(key) {
		// value node
		val, _, err := rlp.SplitString(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid value node: %v", err)
		}
		meta, err := ml.Next()
		if err != nil {
			return nil, fmt.Errorf("invalid value meta: %v", err)
		}

		vn := &valueNode{Value: append([]byte(nil), val...)}
		if len(meta) > 0 {
			vn.meta = append([]byte(nil), meta...)
		}
		return &shortNode{key, vn, flag}, nil
	}

	r, _, err := decodeRef(rest, ml, nl)
	if err != nil {
		return nil, wrapError(err, "val")
	}
	return &shortNode{key, r, flag}, nil
}

func decodeFull(hash *hashNode, buf, elems []byte, ml *metaList, nl *numList) (*fullNode, error) {
	n := &fullNode{flags: nodeFlag{hash: hash}}
	for i := 0; i < 16; i++ {
		cld, rest, err := decodeRef(elems, ml, nl)
		if err != nil {
			return n, wrapError(err, fmt.Sprintf("[%d]", i))
		}
		n.Children[i], elems = cld, rest
	}
	val, _, err := rlp.SplitString(elems)
	if err != nil {
		return n, err
	}
	if len(val) > 0 {
		meta, err := ml.Next()
		if err != nil {
			return nil, fmt.Errorf("invalid value meta: %v", err)
		}

		vn := &valueNode{Value: append([]byte(nil), val...)}
		if len(meta) > 0 {
			vn.meta = append([]byte(nil), meta...)
		}
		n.Children[16] = vn
	}
	return n, nil
}

const hashLen = len(thor.Bytes32{})

func decodeRef(buf []byte, ml *metaList, nl *numList) (node, []byte, error) {
	kind, val, rest, err := rlp.Split(buf)
	if err != nil {
		return nil, buf, err
	}
	switch {
	case kind == rlp.List:
		// 'embedded' node reference. The encoding must be smaller
		// than a hash in order to be valid.
		if size := len(buf) - len(rest); size > hashLen {
			err := fmt.Errorf("oversized embedded node (size is %d bytes, want size < %d)", size, hashLen)
			return nil, buf, err
		}
		n, err := decodeNode(nil, buf, ml, nil)
		return n, rest, err
	case kind == rlp.String && len(val) == 0:
		// empty node
		return nil, rest, nil
	case kind == rlp.String && len(val) == 32:
		cNum, dNum, err := nl.Next()
		if err != nil {
			return nil, nil, fmt.Errorf("invalid commit number: %v", err)
		}
		return &hashNode{append([]byte(nil), val...), cNum, dNum}, rest, nil
	default:
		return nil, nil, fmt.Errorf("invalid RLP string size %d (want 0 or 32)", len(val))
	}
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

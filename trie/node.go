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
		hash []byte
		cNum uint32 // the commit number
	}
	valueNode struct {
		value []byte
		meta  []byte // metadata of the value
	}
)

// EncodeRLP encodes a full node into the consensus RLP format.
func (n *fullNode) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, n.Children)
}

// EncodeRLP encodes a hash node into the consensus RLP format.
func (n *hashNode) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, n.hash)
}

// EncodeRLP encodes a value node into the consensus RLP format.
func (n *valueNode) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, n.value)
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
func (n *shortNode) commitNum() uint32 {
	if n.flags.hash != nil {
		return n.flags.hash.cNum
	}
	return 0
}
func (n *hashNode) commitNum() uint32  { return n.cNum }
func (n *valueNode) commitNum() uint32 { return 0 }

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
	return fmt.Sprintf("<%x> ", n.hash)
}
func (n *valueNode) fstring(ind string) string {
	return fmt.Sprintf("%x ", n.value)
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

	_, content, rest, err := rlp.Split(*ml)
	if err != nil {
		return nil, err
	}

	*ml = rest
	return content, nil
}

// commitNumList is the splitted rlp list of commit numbers.
type commitNumList []byte

// Next returns the commit number of the current child hash node and move to the next none.
// It returns io.EOF if reaches end.
func (cnl *commitNumList) Next() (uint32, error) {
	if cnl == nil {
		return 0, nil
	}
	if len(*cnl) == 0 {
		return 0, io.EOF
	}
	_, _, rest, err := rlp.Split(*cnl)
	if err != nil {
		return 0, err
	}
	var cn uint32
	if err := rlp.DecodeBytes((*cnl)[:len(*cnl)-len(rest)], &cn); err != nil {
		return 0, err
	}

	*cnl = rest
	return cn, nil
}

// decodeTrailing decodes the trailing buffer.
func decodeTrailing(buf []byte) (*metaList, *commitNumList, error) {
	if len(buf) == 0 {
		return nil, nil, nil
	}

	mBuf, rest, err := rlp.SplitList(buf)
	if err != nil {
		return nil, nil, err
	}

	cnBuf, rest, err := rlp.SplitList(rest)
	if err != nil {
		return nil, nil, err
	}
	if len(rest) > 0 {
		return nil, nil, errors.New("encode trailing, unexpected rest bytes")
	}
	return (*metaList)(&mBuf), (*commitNumList)(&cnBuf), nil
}

func encodeTrailing(collapsed node, w io.Writer) error {
	var metaList [][]byte
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
			if len(n.value) > 0 {
				metaList = append(metaList, n.meta)
			}
		}
	}
	collectMeta(collapsed)
	if err := rlp.Encode(w, metaList); err != nil {
		return err
	}

	var cnList []uint32
	switch n := collapsed.(type) {
	case *shortNode:
		if h, ok := n.Val.(*hashNode); ok {
			cnList = append(cnList, h.commitNum())
		}
	case *fullNode:
		for i := 0; i < 16; i++ {
			if h, ok := n.Children[i].(*hashNode); ok {
				cnList = append(cnList, h.commitNum())
			}
		}
	default:
		panic(fmt.Sprintf("encode trailing, unexpected node: %v", n))
	}
	return rlp.Encode(w, cnList)
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
func decodeNode(hash *hashNode, buf []byte, ml *metaList, cnl *commitNumList) (node, error) {
	if len(buf) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	elems, _, err := rlp.SplitList(buf)
	if err != nil {
		return nil, fmt.Errorf("decode error: %v", err)
	}
	switch c, _ := rlp.CountValues(elems); c {
	case 2:
		n, err := decodeShort(hash, buf, elems, ml, cnl)
		return n, wrapError(err, "short")
	case 17:
		n, err := decodeFull(hash, buf, elems, ml, cnl)
		return n, wrapError(err, "full")
	default:
		return nil, fmt.Errorf("invalid number of list elements: %v", c)
	}
}

func decodeShort(hash *hashNode, buf, elems []byte, ml *metaList, cnl *commitNumList) (*shortNode, error) {
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

		vn := &valueNode{value: append([]byte(nil), val...)}
		if len(meta) > 0 {
			vn.meta = append([]byte(nil), meta...)
		}
		return &shortNode{key, vn, flag}, nil
	}

	r, _, err := decodeRef(rest, ml, cnl)
	if err != nil {
		return nil, wrapError(err, "val")
	}
	return &shortNode{key, r, flag}, nil
}

func decodeFull(hash *hashNode, buf, elems []byte, ml *metaList, cnl *commitNumList) (*fullNode, error) {
	n := &fullNode{flags: nodeFlag{hash: hash}}
	for i := 0; i < 16; i++ {
		cld, rest, err := decodeRef(elems, ml, cnl)
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

		vn := &valueNode{value: append([]byte(nil), val...)}
		if len(meta) > 0 {
			vn.meta = append([]byte(nil), meta...)
		}
		n.Children[16] = vn
	}
	return n, nil
}

const hashLen = len(thor.Bytes32{})

func decodeRef(buf []byte, ml *metaList, cnl *commitNumList) (node, []byte, error) {
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
		cn, err := cnl.Next()
		if err != nil {
			return nil, nil, fmt.Errorf("invalid commit number: %v", err)
		}
		return &hashNode{append([]byte(nil), val...), cn}, rest, nil
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

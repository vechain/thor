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
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/lowrlp"
	"github.com/vechain/thor/thor"
)

var NonCryptoNodeHash = thor.BytesToBytes32(bytes.Repeat([]byte{0xff}, 32))
var nonCryptoNodeHashPlaceholder = []byte{0}

var indices = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e", "f", "[17]"}

type node interface {
	fstring(string) string
	cache() (*hashNode, bool, uint16)
	seqNum() uint64
	encode(e *lowrlp.Encoder, nonCrypto bool)
	encodeTrailing(*lowrlp.Encoder)
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
		Hash thor.Bytes32
		seq  uint64 // the sequence number
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
	gen   uint16    // cache generation counter
}

func (n *fullNode) cache() (*hashNode, bool, uint16) { return n.flags.hash, n.flags.dirty, n.flags.gen }
func (n *shortNode) cache() (*hashNode, bool, uint16) {
	return n.flags.hash, n.flags.dirty, n.flags.gen
}
func (n *hashNode) cache() (*hashNode, bool, uint16)  { return nil, true, 0 }
func (n *valueNode) cache() (*hashNode, bool, uint16) { return nil, true, 0 }

func (n *fullNode) seqNum() uint64 {
	if n.flags.hash != nil {
		return n.flags.hash.seq
	}
	return 0
}

func (n *shortNode) seqNum() uint64 {
	if n.flags.hash != nil {
		return n.flags.hash.seq
	}
	return 0
}

func (n *hashNode) seqNum() uint64  { return n.seq }
func (n *valueNode) seqNum() uint64 { return 0 }

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
	return fmt.Sprintf("<%v> ", n.Hash)
}
func (n *valueNode) fstring(ind string) string {
	return fmt.Sprintf("%x ", n.Value)
}

// trailing is the splitted rlp list of extra data of the trie node.
type trailing []byte

func (t *trailing) next() ([]byte, error) {
	if t == nil {
		return nil, nil
	}
	if len(*t) == 0 {
		return nil, io.EOF
	}

	content, rest, err := rlp.SplitString(*t)
	if err != nil {
		return nil, err
	}

	*t = rest
	return content, nil
}

// NextSeq decodes the current list element to seq number and move to the next one.
// It returns io.EOF if reaches end.
func (t *trailing) NextSeq() (seq uint64, err error) {
	content, err := t.next()
	if err != nil {
		return 0, err
	}
	if len(content) > 8 {
		return 0, errors.New("encoded seq too long")
	}

	for _, b := range content {
		seq <<= 8
		seq |= uint64(b)
	}
	return
}

// NextMeta returns the current list element as leaf metadata and move to the next one.
// It returns io.EOF if reaches end.
func (t *trailing) NextMeta() ([]byte, error) {
	return t.next()
}

func mustDecodeNode(hash *hashNode, buf []byte, cacheGen uint16) node {
	_, _, rest, err := rlp.Split(buf)
	if err != nil {
		panic(fmt.Sprintf("node %v: %v", hash.Hash, err))
	}
	trailing := (*trailing)(&rest)
	if len(rest) == 0 {
		trailing = nil
	}
	buf = buf[:len(buf)-len(rest)]
	n, err := decodeNode(hash, buf, trailing, cacheGen)
	if err != nil {
		panic(fmt.Sprintf("node %v: %v", hash.Hash, err))
	}
	if trailing != nil && len(*trailing) != 0 {
		panic(fmt.Sprintf("node %v: trailing buffer not fully consumed", hash.Hash))
	}
	return n
}

// decodeNode parses the RLP encoding of a trie node.
func decodeNode(hash *hashNode, buf []byte, trailing *trailing, cacheGen uint16) (node, error) {
	if len(buf) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	elems, _, err := rlp.SplitList(buf)
	if err != nil {
		return nil, fmt.Errorf("decode error: %v", err)
	}
	switch c, _ := rlp.CountValues(elems); c {
	case 2:
		n, err := decodeShort(hash, buf, elems, trailing, cacheGen)
		return n, wrapError(err, "short")
	case 17:
		n, err := decodeFull(hash, buf, elems, trailing, cacheGen)
		return n, wrapError(err, "full")
	default:
		return nil, fmt.Errorf("invalid number of list elements: %v", c)
	}
}

func decodeShort(hash *hashNode, buf, elems []byte, trailing *trailing, cacheGen uint16) (*shortNode, error) {
	kbuf, rest, err := rlp.SplitString(elems)
	if err != nil {
		return nil, err
	}
	flag := nodeFlag{hash: hash, gen: cacheGen}
	key := compactToHex(kbuf)
	if hasTerm(key) {
		// value node
		val, _, err := rlp.SplitString(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid value node: %v", err)
		}
		meta, err := trailing.NextMeta()
		if err != nil {
			return nil, fmt.Errorf("invalid value meta: %v", err)
		}

		vn := &valueNode{Value: append([]byte(nil), val...)}
		if len(meta) > 0 {
			vn.meta = append([]byte(nil), meta...)
		}
		return &shortNode{key, vn, flag}, nil
	}

	r, _, err := decodeRef(rest, trailing, cacheGen)
	if err != nil {
		return nil, wrapError(err, "val")
	}
	return &shortNode{key, r, flag}, nil
}

func decodeFull(hash *hashNode, buf, elems []byte, trailing *trailing, cacheGen uint16) (*fullNode, error) {
	n := &fullNode{flags: nodeFlag{hash: hash, gen: cacheGen}}
	for i := 0; i < 16; i++ {
		cld, rest, err := decodeRef(elems, trailing, cacheGen)
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
		meta, err := trailing.NextMeta()
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

func decodeRef(buf []byte, trailing *trailing, cacheGen uint16) (node, []byte, error) {
	kind, val, rest, err := rlp.Split(buf)
	if err != nil {
		return nil, buf, err
	}
	if kind == rlp.List {
		// 'embedded' node reference. The encoding must be smaller
		// than a hash in order to be valid.
		if size := len(buf) - len(rest); size > hashLen {
			err := fmt.Errorf("oversized embedded node (size is %d bytes, want size < %d)", size, hashLen)
			return nil, buf, err
		}
		n, err := decodeNode(nil, buf, trailing, cacheGen)
		return n, rest, err
	}
	// string kind
	valLen := len(val)
	if valLen == 0 {
		// empty node
		return nil, rest, nil
	}
	seq, err := trailing.NextSeq()
	if err != nil {
		return nil, nil, fmt.Errorf("invalid seq number: %v", err)
	}
	if valLen == 32 {
		return &hashNode{Hash: thor.BytesToBytes32(val), seq: seq}, rest, nil
	}
	if valLen == 1 && val[0] == nonCryptoNodeHashPlaceholder[0] {
		return &hashNode{Hash: NonCryptoNodeHash, seq: seq}, rest, nil
	}
	return nil, nil, fmt.Errorf("invalid RLP string size %d (want 0, 1 or 32)", len(val))
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

// VerifyNodeHash verifies the hash of the node blob (trailing excluded).
func VerifyNodeHash(blob, expectedHash []byte) (bool, error) {
	// strip the trailing
	_, _, trailing, err := rlp.Split(blob)
	if err != nil {
		return false, err
	}

	node := blob[:len(blob)-len(trailing)]
	have := thor.Blake2b(node)
	return bytes.Equal(expectedHash, have.Bytes()), nil
}

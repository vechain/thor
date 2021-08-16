// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

// path64 uses uint64 to present the trie node path and follows the order of trie node iteration.
// Paths longer than 15 will be trimmed to 15.
type path64 uint64

// newPath64 convert the trie node path into path64.
func newPath64(path []byte) path64 {
	n := len(path)
	if n > 15 {
		n = 15
	}

	var p path64
	for i := 0; i < 15; i++ {
		if i < n {
			p |= path64(path[i])
		}
		p <<= 4
	}
	return p | path64(n)
}

// Append appends a path element and returns the new path.
func (p path64) Append(e byte) path64 {
	pathLen := p.Len()
	if pathLen == 15 {
		return p
	}
	shift := uint(60 - pathLen*4)
	return (p | (path64(e) << shift)) + 1
}

// Len returns the length of the path.
func (p path64) Len() int {
	return int(p & 0xf)
}

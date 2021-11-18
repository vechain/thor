// Copyright (c) 2021 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package trie

// PartitionFactor is factor to partition trie nodes bases on commit number.
type PartitionFactor uint32

// Range returns the commit number range (closed interval) of the given partition id.
func (pf PartitionFactor) Range(pid uint32) (uint32, uint32) {
	if pid == 0 {
		return 0, 0
	}
	return (pid-1)*uint32(pf) + 1, pid * uint32(pf)
}

// Which returns the partition id for the given commit number.
func (pf PartitionFactor) Which(cn uint32) (pid uint32) {
	// convert to uint64 to avoid overflow
	return uint32((uint64(cn) + uint64(pf) - 1) / uint64(pf))
}

// encodePath encodes the path into 4-bytes aligned compact format.
// The encoded paths are in lexicographical order even with suffix appended.
func encodePath(dst []byte, path []byte) []byte {
	if len(path) == 0 {
		return append(dst, 0, 0, 0, 0)
	}
	for i := 0; i < len(path); i += 7 {
		dst = appendUint32(dst, encodePath32(path[i:]))
	}
	return dst
}

// encodePath32 encodes at most 7 path elements into uint32.
func encodePath32(path []byte) uint32 {
	n := len(path)
	if n > 7 {
		n = 8 // means have subsequent path elements.
	}

	var v uint32
	for i := 0; i < 7; i++ {
		if i < n {
			v |= uint32(path[i])
		}
		v <<= 4
	}
	return v | uint32(n)
}

func appendUint32(b []byte, v uint32) []byte {
	return append(b,
		byte(v>>24),
		byte(v>>16),
		byte(v>>8),
		byte(v),
	)
}

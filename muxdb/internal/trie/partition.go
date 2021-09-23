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

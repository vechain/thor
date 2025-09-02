// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package solidity

// toWordSize converts bytes length to word size, here we define a simplified rule.
// Any length larger than 32 will be considered as 2 words. Since we do not do 32 bytes storage in native package.
func toWordSize(length int) uint64 {
	if length > 32 {
		return 2
	}

	return 1
}

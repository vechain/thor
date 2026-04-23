// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package thor

import "encoding/binary"

// ChainID returns the 2-byte EIP-155-compatible chain id derived from the
// last two bytes of the 32-byte genesis block ID, interpreted as big-endian
// uint16 and widened to uint64 for downstream *big.Int carriers.
//
// This value is the Interstellar-and-later CHAIN_ID. Concrete values:
//
//	mainnet genesis 0x...1b4a -> 6986  (0x1b4a)
//	testnet genesis 0x...b127 -> 45351 (0xb127)
//
// Callers that need fork-aware behaviour must wrap this with
// thor.IsForked(blockNum, forkConfig.INTERSTELLAR); this function itself is
// fork-agnostic.
func ChainID(genesisID Bytes32) uint64 {
	return uint64(binary.BigEndian.Uint16(genesisID[30:32]))
}

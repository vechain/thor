// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/thor"
)

// BlockRef is block reference.
type BlockRef [8]byte

// Number extracts block number.
func (br BlockRef) Number() uint32 {
	return binary.BigEndian.Uint32(br[:])
}

// NewBlockRef create block reference with block number.
func NewBlockRef(blockNum uint32) (br BlockRef) {
	binary.BigEndian.PutUint32(br[:], blockNum)
	return
}

// NewBlockRefFromID create block reference from block id.
func NewBlockRefFromID(blockID thor.Bytes32) (br BlockRef) {
	copy(br[:], blockID[:])
	return
}

// NewBlockRefFromHex creates a BlockRef from a hex string.
func NewBlockRefFromHex(hexStr string) (BlockRef, error) {
	var br BlockRef

	// Decode hex string
	bytes, err := hexutil.Decode(hexStr)
	if err != nil {
		return br, fmt.Errorf("invalid hex: %v", err)
	}

	// Check length
	if len(bytes) != 8 {
		return br, fmt.Errorf("invalid length: expected 8 bytes, got %d", len(bytes))
	}

	// Copy bytes to BlockRef
	copy(br[:], bytes)
	return br, nil
}

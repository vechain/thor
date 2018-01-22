package tx

import (
	"encoding/binary"

	"github.com/vechain/thor/thor"
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
func NewBlockRefFromID(blockID thor.Hash) (br BlockRef) {
	copy(br[:], blockID[:])
	return
}

package tx

import (
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/cry"
)

// Receipts slice of receipts.
type Receipts []*Receipt

// RootHash computes merkle root hash of receipts.
func (rs Receipts) RootHash() cry.Hash {
	if len(rs) == 0 {
		// optimized
		return EmptyRoot
	}
	return cry.Hash(types.DeriveSha(derivableReceipts(rs)))
}

// implements DerivableList
type derivableReceipts Receipts

func (rs derivableReceipts) Len() int {
	return len(rs)
}
func (rs derivableReceipts) GetRlp(i int) []byte {
	data, err := rlp.EncodeToBytes(rs[i])
	if err != nil {
		panic(err)
	}
	return data
}

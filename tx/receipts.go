package tx

import (
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

// Receipts slice of receipts.
type Receipts []*Receipt

// RootHash computes merkle root hash of receipts.
func (rs Receipts) RootHash() thor.Hash {
	if len(rs) == 0 {
		// optimized
		return emptyRoot
	}
	return thor.Hash(types.DeriveSha(derivableReceipts(rs)))
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

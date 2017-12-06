package receipt

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/vecore/cry"
)

// Receipt represents the results of a transaction.
type Receipt struct {
	// status of tx execution
	Status uint
	// which clause caused tx failure
	BadClauseIndex uint
	// gas used by this tx
	GasUsed *big.Int
	// logs produced
	Logs []*Log
}

// Receipts slice of receipts.
type Receipts []*Receipt

// RootHash computes merkle root hash of receipts.
func (rs Receipts) RootHash() cry.Hash {
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

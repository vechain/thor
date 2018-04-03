package tx

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
)

// Receipt represents the results of a transaction.
type Receipt struct {
	// gas used by this tx
	GasUsed uint64
	// the one who payed for gas
	GasPayer thor.Address
	// energy reward given to block proposer
	Reward *big.Int
	// if the tx reverted
	Reverted bool
	// outputs of clauses in tx
	Outputs []*Output
}

// Output output of clause execution.
type Output struct {
	// logs produced by the clause
	Logs []*Log
}

// Receipts slice of receipts.
type Receipts []*Receipt

// RootHash computes merkle root hash of receipts.
func (rs Receipts) RootHash() thor.Bytes32 {
	if len(rs) == 0 {
		// optimized
		return emptyRoot
	}
	return thor.Bytes32(types.DeriveSha(derivableReceipts(rs)))
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

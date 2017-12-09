package tx

import (
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/cry"
)

// Transactions a slice of transactions.
type Transactions []*Transaction

// Copy makes a shallow copy.
func (txs Transactions) Copy() Transactions {
	return append(Transactions(nil), txs...)
}

// RootHash computes merkle root hash of transactions.
func (txs Transactions) RootHash() cry.Hash {
	return cry.Hash(types.DeriveSha(derivableTxs(txs)))
}

// implements types.DerivableList
type derivableTxs Transactions

func (txs derivableTxs) Len() int {
	return len(txs)
}

func (txs derivableTxs) GetRlp(i int) []byte {
	data, err := rlp.EncodeToBytes(txs[i])
	if err != nil {
		panic(err)
	}
	return data
}

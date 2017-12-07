package tx

import (
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/cry"
)

// Transactions a slice of transactions.
type Transactions []*Transaction

// Copy returns a shallow copy.
func (txs Transactions) Copy() Transactions {
	return append(Transactions(nil), txs...)
}

// RootHash computes merkle root hash of transactions.
func (txs Transactions) RootHash() cry.Hash {
	return cry.Hash(types.DeriveSha(derivableTxs(txs)))
}

// DecodeRLP implements rlp.Decoder.
func (txs *Transactions) DecodeRLP(s *rlp.Stream) error {
	var ds []Decoder
	if err := s.Decode(&ds); err != nil {
		return err
	}
	*txs = Transactions{}
	for _, txd := range ds {
		*txs = append(*txs, txd.Result)
	}
	return nil
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

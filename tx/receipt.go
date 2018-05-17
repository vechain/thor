// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package tx

import (
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

// Receipt represents the results of a transaction.
type Receipt struct {
	// gas used by this tx
	GasUsed uint64
	// the one who paid for gas
	GasPayer thor.Address
	// energy paid for used gas
	Paid *big.Int
	// energy reward given to block proposer
	Reward *big.Int
	// if the tx reverted
	Reverted bool
	// outputs of clauses in tx
	Outputs []*Output
}

// Output output of clause execution.
type Output struct {
	// events produced by the clause
	Events Events
	// transfer occurred in clause
	Transfers Transfers
}

// Receipts slice of receipts.
type Receipts []*Receipt

// RootHash computes merkle root hash of receipts.
func (rs Receipts) RootHash() thor.Bytes32 {
	if len(rs) == 0 {
		// optimized
		return emptyRoot
	}
	return trie.DeriveRoot(derivableReceipts(rs))
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

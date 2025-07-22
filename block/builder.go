// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"math/big"
	"slices"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

// Builder to make it easy to build a block object.
type Builder struct {
	headerBody headerBody
	txs        tx.Transactions
	evidences  *[][]Header
}

// ParentID set parent id.
func (b *Builder) ParentID(id thor.Bytes32) *Builder {
	b.headerBody.ParentID = id
	return b
}

// Timestamp set timestamp.
func (b *Builder) Timestamp(ts uint64) *Builder {
	b.headerBody.Timestamp = ts
	return b
}

// TotalScore set total score.
func (b *Builder) TotalScore(score uint64) *Builder {
	b.headerBody.TotalScore = score
	return b
}

// GasLimit set gas limit.
func (b *Builder) GasLimit(limit uint64) *Builder {
	b.headerBody.GasLimit = limit
	return b
}

// GasUsed set gas used.
func (b *Builder) GasUsed(used uint64) *Builder {
	b.headerBody.GasUsed = used
	return b
}

// Beneficiary set recipient of reward.
func (b *Builder) Beneficiary(addr thor.Address) *Builder {
	b.headerBody.Beneficiary = addr
	return b
}

// StateRoot set state root.
func (b *Builder) StateRoot(hash thor.Bytes32) *Builder {
	b.headerBody.StateRoot = hash
	return b
}

// ReceiptsRoot set receipts root.
func (b *Builder) ReceiptsRoot(hash thor.Bytes32) *Builder {
	b.headerBody.ReceiptsRoot = hash
	return b
}

// Transaction add a transaction.
func (b *Builder) Transaction(tx *tx.Transaction) *Builder {
	b.txs = append(b.txs, tx)
	return b
}

// TransactionFeatures set supported transaction features
func (b *Builder) TransactionFeatures(features tx.Features) *Builder {
	b.headerBody.TxsRootFeatures.Features = features
	return b
}

// Alpha set the alpha.
func (b *Builder) Alpha(alpha []byte) *Builder {
	b.headerBody.Extension.Alpha = slices.Clone(alpha)
	return b
}

// COM enables COM.
func (b *Builder) COM() *Builder {
	b.headerBody.Extension.COM = true
	return b
}

// BaseFee sets base fee.
func (b *Builder) BaseFee(baseFee *big.Int) *Builder {
	b.headerBody.Extension.BaseFee = baseFee
	return b
}

// Evidence sets evidence.
func (b *Builder) Evidence(evidences *[][]Header) *Builder {
	b.evidences = evidences
	blockEvIDs := make([][]thor.Bytes32, len(*evidences))
	for blkIdx, blockEv := range *evidences {
		evidence := make([]thor.Bytes32, len(blockEv))
		for idx, ev := range blockEv {
			evidence[idx] = ev.ID()
		}
		blockEvIDs[blkIdx] = evidence
	}
	b.headerBody.Extension.Evidence = &blockEvIDs
	return b
}

// Build build a block object.
func (b *Builder) Build() *Block {
	header := Header{body: b.headerBody}
	header.body.TxsRootFeatures.Root = b.txs.RootHash()

	return &Block{
		header:    &header,
		txs:       b.txs,
		evidences: b.evidences,
	}
}

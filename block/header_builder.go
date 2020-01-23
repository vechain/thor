package block

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vrf"
)

// HeaderBuilder ...
type HeaderBuilder struct {
	headerBody headerBody
	txs        tx.Transactions
}

// ParentID set parent id.
func (b *HeaderBuilder) ParentID(id thor.Bytes32) *HeaderBuilder {
	b.headerBody.ParentID = id
	return b
}

// Timestamp set timestamp.
func (b *HeaderBuilder) Timestamp(ts uint64) *HeaderBuilder {
	b.headerBody.Timestamp = ts
	return b
}

// TotalScore set total score.
func (b *HeaderBuilder) TotalScore(score uint64) *HeaderBuilder {
	b.headerBody.TotalScore = score
	return b
}

// GasLimit set gas limit.
func (b *HeaderBuilder) GasLimit(limit uint64) *HeaderBuilder {
	b.headerBody.GasLimit = limit
	return b
}

// GasUsed set gas used.
func (b *HeaderBuilder) GasUsed(used uint64) *HeaderBuilder {
	b.headerBody.GasUsed = used
	return b
}

// Beneficiary set recipient of reward.
func (b *HeaderBuilder) Beneficiary(addr thor.Address) *HeaderBuilder {
	b.headerBody.Beneficiary = addr
	return b
}

// StateRoot set state root.
func (b *HeaderBuilder) StateRoot(hash thor.Bytes32) *HeaderBuilder {
	b.headerBody.StateRoot = hash
	return b
}

// ReceiptsRoot set receipts root.
func (b *HeaderBuilder) ReceiptsRoot(hash thor.Bytes32) *HeaderBuilder {
	b.headerBody.ReceiptsRoot = hash
	return b
}

// Transaction add a transaction.
func (b *HeaderBuilder) Transaction(tx *tx.Transaction) *HeaderBuilder {
	b.txs = append(b.txs, tx)
	return b
}

// TransactionFeatures set supported transaction features
func (b *HeaderBuilder) TransactionFeatures(features tx.Features) *HeaderBuilder {
	b.headerBody.TxsRootFeatures.Features = features
	return b
}

// // Committee ...
// func (b *HeaderBuilder) Committee(c []uint8) *HeaderBuilder {
// 	// b.headerBody.Committee = append([]uint8(nil), c...)
// 	b.headerBody.Committee = c
// 	return b
// }

// VrfProofs ...
func (b *HeaderBuilder) VrfProofs(proofs []*vrf.Proof) *HeaderBuilder {
	// for _, p := range proofs {
	// 	b.headerBody.VrfProofs = append(b.headerBody.VrfProofs, p.Copy())
	// }
	b.headerBody.VrfProofs = proofs
	return b
}

// SigOnBlockSummary ...
func (b *HeaderBuilder) SigOnBlockSummary(sig []byte) *HeaderBuilder {
	// b.headerBody.SigOnBlockSummary = append([]byte(nil), sig...)
	b.headerBody.SigOnBlockSummary = sig
	return b
}

// SigOnEndorsement ...
func (b *HeaderBuilder) SigOnEndorsement(sigs [][]byte) *HeaderBuilder {
	// for _, sig := range sigs {
	// 	s := append([]byte(nil), sig...)
	// 	b.headerBody.SigOnEndorsement = append(b.headerBody.SigOnEndorsement, s)
	// }
	b.headerBody.SigOnEndorsement = sigs
	return b
}

// Build build a block object.
func (b *HeaderBuilder) Build() *Header {
	header := Header{body: b.headerBody}
	header.body.TxsRootFeatures.Root = b.txs.RootHash()
	return &header
}

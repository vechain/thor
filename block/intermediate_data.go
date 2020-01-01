package block

import (
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/tx"
	"github.com/vechain/thor/vrf"
)

// Summary block summary
type Summary struct {
	ParentID thor.Bytes32
	TxRoot   thor.Bytes32
	RoundNum uint32

	Signature []byte
}

// Endorsement endorsement
type Endorsement struct {
	BlockSummary Summary
	VrfPubkey    vrf.PublicKey
	VrfProof     vrf.Proof

	Signature []byte
}

// TxSet transaction set
type TxSet struct {
	TxRoot thor.Bytes32
	Txs    tx.Transactions

	Signature []byte
}

// Endorsements endorsement array
type Endorsements []*Endorsement

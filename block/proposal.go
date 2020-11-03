// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package block

import (
	"encoding/binary"

	"github.com/vechain/thor/thor"
)

// Proposal is block proposal.
type Proposal struct {
	ParentID  thor.Bytes32
	TxsRoot   thor.Bytes32
	GasLimit  uint64
	Timestamp uint64
}

// NewProposal creates a new proposal.
func NewProposal(parentID, txsRoot thor.Bytes32, gasLimit, timestamp uint64) *Proposal {
	return &Proposal{
		ParentID:  parentID,
		TxsRoot:   txsRoot,
		GasLimit:  gasLimit,
		Timestamp: timestamp,
	}
}

// Number returns number of the proposal.
func (p *Proposal) Number() uint32 {
	return Number(p.ParentID) + 1
}

// Hash returns the hash of the proposal body.
func (p *Proposal) Hash() (hash thor.Bytes32) {
	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b, p.GasLimit)
	binary.BigEndian.PutUint64(b[8:], p.Timestamp)

	// [parentID + txsRoot + Gaslimit + Timestamp]
	return thor.Blake2b(p.ParentID.Bytes(), p.TxsRoot.Bytes(), b)
}

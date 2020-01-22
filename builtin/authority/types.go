// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package authority

import (
	"github.com/vechain/thor/thor"
)

type (
	entry struct {
		Endorsor thor.Address
		Identity thor.Bytes32
		Active   bool
		Prev     *thor.Address `rlp:"nil"`
		Next     *thor.Address `rlp:"nil"`
	}

	// Candidate candidate of block proposer.
	Candidate struct {
		NodeMaster thor.Address
		Endorsor   thor.Address
		Identity   thor.Bytes32
		Active     bool
	}
)

// IsEmpty returns whether the entry can be treated as empty.
func (e *entry) IsEmpty() bool {
	return e.Endorsor.IsZero() &&
		e.Identity.IsZero() &&
		!e.Active &&
		e.Prev == nil &&
		e.Next == nil
}

func (e *entry) IsLinked() bool {
	return e.Prev != nil || e.Next != nil
}

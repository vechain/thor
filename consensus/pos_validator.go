// Copyright (c) 2025 The VeChainThor developers
//
// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package consensus

import (
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/tx"
)

func (c *Consensus) validateStakingProposer(header *block.Header, parent *block.Header, st *state.State) error {
	panic("implement me")
}

func (c *Consensus) stakerReceiptsHandler() func(receipts tx.Receipts) error {
	panic("implement me")
}

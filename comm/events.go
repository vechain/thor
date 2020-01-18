// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"context"

	"github.com/vechain/thor/block"
)

// NewBlockEvent event emitted when received block announcement.
type NewBlockEvent struct {
	*block.Block
}

// NewBlockSummaryEvent ...
type NewBlockSummaryEvent struct {
	*block.Summary
}

// NewTxSetEvent ...
type NewTxSetEvent struct {
	*block.TxSet
}

// NewEndorsementEvent ...
type NewEndorsementEvent struct {
	*block.Endorsement
}

// NewHeaderEvent ...
type NewHeaderEvent struct{
	*block.Header
}

// HandleBlockStream to handle the stream of downloaded blocks in sync process.
type HandleBlockStream func(ctx context.Context, stream <-chan *block.Block) error

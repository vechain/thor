// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package comm

import (
	"context"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/p2p/discover"
)

// NewBlockEvent event emitted when received block announcement.
type NewBlockEvent struct {
	*block.Block
	// Origin is the peer the block was received from. Events with a zero
	// Origin share one per-peer cap in the future block cache.
	Origin discover.NodeID
}

// HandleBlockStream to handle the stream of downloaded blocks in sync process.
type HandleBlockStream func(ctx context.Context, stream <-chan *block.Block) error

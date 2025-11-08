// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"

	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/tx"
)

// Consensus defines the interface for consensus processing
type Consensus interface {
	Process(parentSummary *chain.BlockSummary, blk *block.Block, nowTimestamp uint64, blockConflicts uint32) (*state.Stage, tx.Receipts, error)
}

// Packer defines the interface for packing blocks
type Packer interface {
	Schedule(parent *chain.BlockSummary, nowTimestamp uint64) (flow *packer.Flow, posActive bool, err error)
	SetTargetGasLimit(gl uint64)
}

// CommunicatorEngine defines the interface for p2p communication
type Communicator interface {
	Sync(ctx context.Context, handler comm.HandleBlockStream)
	SubscribeBlock(ch chan *comm.NewBlockEvent) event.Subscription
	BroadcastBlock(blk *block.Block)
	PeerCount() int
	Synced() <-chan struct{}
}

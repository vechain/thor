// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"

	"github.com/ethereum/go-ethereum/event"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/packer"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/txpool"
)

// ConsensusEngine defines the interface for consensus processing
type ConsensusEngine interface {
	Process(parentSummary *chain.BlockSummary, blk *block.Block, nowTimestamp uint64, blockConflicts uint32) (*state.Stage, tx.Receipts, error)
}

// PackerEngine defines the interface for packing blocks
type PackerEngine interface {
	Schedule(parent *chain.BlockSummary, nowTimestamp uint64) (flow *packer.Flow, posActive bool, err error)
	SetTargetGasLimit(gl uint64)
}

// CommunicatorEngine defines the interface for p2p communication
type CommunicatorEngine interface {
	Sync(ctx context.Context, handler comm.HandleBlockStream)
	SubscribeBlock(ch chan *comm.NewBlockEvent) event.Subscription
	BroadcastBlock(blk *block.Block)
	PeerCount() int
	Synced() <-chan struct{}
}

type RepositoryEngine interface {
	GetMaxBlockNum() (uint32, error)
	ScanConflicts(blockNum uint32) (uint32, error)
	GetBlockSummary(id thor.Bytes32) (*chain.BlockSummary, error)
	IsNotFound(error) bool
	BestBlockSummary() *chain.BlockSummary
	NewChain(head thor.Bytes32) *chain.Chain
	AddBlock(blk *block.Block, receipts tx.Receipts, conflicts uint32, isTrunk bool) error
	GetBlock(id thor.Bytes32) (*block.Block, error)
	GetBlockReceipts(id thor.Bytes32) (tx.Receipts, error)
	NewTicker() co.Waiter
	GetConflicts(blockNum uint32) ([]thor.Bytes32, error)
	ChainTag() byte
	GenesisBlock() *block.Block
	ScanHeads(from uint32) ([]thor.Bytes32, error)
	GetBlockTransactions(id thor.Bytes32) (tx.Transactions, error)
}

type BFTEngine interface {
	Accepts(parentID thor.Bytes32) (bool, error)
	Select(header *block.Header) (bool, error)
	CommitBlock(header *block.Header, isPacking bool) error
	ShouldVote(parentID thor.Bytes32) (bool, error)
}

type TxPoolEngine interface {
	Fill(txs tx.Transactions)
	Add(newTx *tx.Transaction) error
	SubscribeTxEvent(ch chan *txpool.TxEvent) event.Subscription
	Executables() tx.Transactions
	Remove(txHash thor.Bytes32, txID thor.Bytes32) bool
	Close()
	AddLocal(newTx *tx.Transaction) error
	Get(id thor.Bytes32) *tx.Transaction
	StrictlyAdd(newTx *tx.Transaction) error
	Dump() tx.Transactions
	Len() int
}

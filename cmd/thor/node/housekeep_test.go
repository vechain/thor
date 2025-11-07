// Copyright (c) 2025 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/event"
	"github.com/stretchr/testify/assert"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/cache"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/comm"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
)

// Mock implementations for testing
type mockCommunicator struct {
	peerCount       int
	broadcastCalled bool
	broadcastBlock  *block.Block
}

func (m *mockCommunicator) BroadcastBlock(blk *block.Block) {
	m.broadcastCalled = true
	m.broadcastBlock = blk
}

func (m *mockCommunicator) PeerCount() int {
	return m.peerCount
}

func (m *mockCommunicator) Sync(ctx context.Context, handler comm.HandleBlockStream) {
}

func (m *mockCommunicator) SubscribeBlock(ch chan *comm.NewBlockEvent) event.Subscription {
	// Return a simple mock subscription
	return &mockSubscription{}
}

func (m *mockCommunicator) Synced() <-chan struct{} {
	return make(chan struct{})
}

// Simple mock subscription
type mockSubscription struct{}

func (m *mockSubscription) Unsubscribe() {}
func (m *mockSubscription) Err() <-chan error {
	return make(chan error)
}

type mockBFT struct{}

func (m *mockBFT) Accepts(parentID thor.Bytes32) (bool, error) {
	return true, nil
}

func (m *mockBFT) Select(header *block.Header) (bool, error) {
	return true, nil
}

func (m *mockBFT) CommitBlock(header *block.Header, isPacking bool) error {
	return nil
}

func (m *mockBFT) ShouldVote(parentID thor.Bytes32) (bool, error) {
	return true, nil
}

func (m *mockBFT) Finalized() thor.Bytes32 {
	return thor.Bytes32{}
}

func (m *mockBFT) Justified() (thor.Bytes32, error) {
	return thor.Bytes32{}, nil
}

type mockConsensus struct {
	stager *state.Stage
}

func newMockConsensus() *mockConsensus {
	newState := state.New(muxdb.NewMem(), trie.Root{})
	stage, err := newState.Stage(trie.Version{Major: 1})
	if err != nil {
		panic(err)
	}
	return &mockConsensus{
		stager: stage,
	}
}

func (m *mockConsensus) Process(
	parentSummary *chain.BlockSummary,
	blk *block.Block,
	nowTimestamp uint64,
	blockConflicts uint32,
) (*state.Stage, tx.Receipts, error) {
	return m.stager, nil, nil
}

func (m *mockConsensus) NewRuntimeForReplay(header *block.Header, skipValidation bool) (*runtime.Runtime, error) {
	return nil, nil
}

type mockableNode struct {
	*Node
}

func setupTestNodeForHousekeeping(t *testing.T) (*mockableNode, *mockCommunicator) {
	// Create test accounts
	accounts := createDevAccounts(t, 3)

	// Create test database
	db := muxdb.NewMem()

	// Create test chain
	chain, err := createChain(db, accounts)
	assert.Nil(t, err)

	// Create mock
	mockComm := &mockCommunicator{peerCount: 1}
	mockBFT := &mockBFT{}
	mockCons := newMockConsensus()

	nodeOptions := Options{
		SkipLogs: true,
	}

	// Create original node
	originalNode := &Node{
		cons:       mockCons,
		repo:       chain.Repo(),
		comm:       mockComm,
		forkConfig: &thor.NoFork,
		bft:        mockBFT,
		newBlockCh: make(chan *comm.NewBlockEvent, 1),
		options:    nodeOptions,
	}

	originalNode.futureBlocksCache = cache.NewRandCache(32)

	// Wrap in test node
	testNode := &mockableNode{Node: originalNode}

	return testNode, mockComm
}

func TestNode_HouseKeeping_Newblock(t *testing.T) {
	tests := []struct {
		name       string
		setupBlock func(node *mockableNode) *block.Block
		assertFunc func(t *testing.T, values map[string]any)
	}{
		{
			name: "successful trunk block processing",
			setupBlock: func(node *mockableNode) *block.Block {
				// Create a block that should be processed successfully
				parentBlock := node.repo.BestBlockSummary()

				builder := new(block.Builder)
				parentID := thor.Bytes32{}
				if parentBlock != nil {
					parentID = parentBlock.Header.ID()
				}

				header := builder.
					ParentID(parentID).
					Timestamp(uint64(time.Now().Unix())).
					GasLimit(10000000).
					TotalScore(11).
					Build().Header()

				trunkBL := block.Compose(header, nil)
				return trunkBL
			},
			assertFunc: func(t *testing.T, values map[string]any) {
				node := values["node"].(*mockableNode)
				mockComm := node.comm.(*mockCommunicator)
				assert.True(t, mockComm.broadcastCalled, "Block should have been broadcast")
				assert.NotEmpty(t, mockComm.broadcastBlock, "Broadcasted block should not be nil")
			},
		},
		{
			name: "parent missing error handling",
			setupBlock: func(node *mockableNode) *block.Block {
				return createTestBlock(thor.MustParseBytes32("0x0000000100000000000000000000000000000000000000000000000000000000"))
			},
			assertFunc: func(t *testing.T, values map[string]any) {
				node := values["node"].(*mockableNode)
				assert.Equal(t, 0, node.futureBlocksCache.Len(), "Future blocks cache should be empty")
			},
		},
		{
			name: "parent in future blocks cache and parent missing error handling",
			setupBlock: func(node *mockableNode) *block.Block {
				// Create a block whose parent is in the future blocks cache
				// bestBlock := node.repo.BestBlockSummary()
				cacheBlock := createTestBlock(thor.MustParseBytes32("0x0000000100000000000000000000000000000000000000000000000000000000"))
				node.futureBlocksCache.Set(cacheBlock.Header().ID(), cacheBlock)

				builder := new(block.Builder)
				header := builder.
					ParentID(cacheBlock.Header().ID()).
					Timestamp(uint64(time.Now().Unix())).
					GasLimit(10000000).
					TotalScore(11).
					Build().Header()

				newBlock := block.Compose(header, nil)
				return newBlock
			},
			assertFunc: func(t *testing.T, values map[string]any) {
				node := values["node"].(*mockableNode)
				assert.Equal(t, 2, node.futureBlocksCache.Len(), "Future blocks cache should contain 2 blocks")
				assert.True(t,
					node.futureBlocksCache.Contains(thor.MustParseBytes32("0x0000000200000000000000000000000000000000000000000000000000000000")),
					"Future blocks cache should contain the parent block")
				assert.True(t,
					node.futureBlocksCache.Contains(thor.MustParseBytes32("0x0000000300000000000000000000000000000000000000000000000000000000")),
					"Future blocks cache should contain the parent block")
			},
		},
		{
			name: "temporaryUnprocessable block handling",
			setupBlock: func(node *mockableNode) *block.Block {
				// Create a block whose parent is in the future blocks cache
				newParentID, _ := thor.ParseBytes32("0x0000000a00000000000000000000000000000000000000000000000000000000") // block number is 10

				builder := new(block.Builder)
				header := builder.
					ParentID(newParentID).
					Timestamp(uint64(time.Now().Unix())).
					GasLimit(10000000).
					TotalScore(11).
					Build().Header()

				parentBlock := block.Compose(header, nil)
				node.futureBlocksCache.Set(parentBlock.Header().ID(), parentBlock)

				builder2 := new(block.Builder)
				header2 := builder2.
					ParentID(parentBlock.Header().ID()).
					Timestamp(uint64(time.Now().Unix())).
					GasLimit(10000000).
					TotalScore(11).
					Build().Header()

				newblock := block.Compose(header2, nil)

				return newblock
			},
			assertFunc: func(t *testing.T, values map[string]any) {
				node := values["node"].(*mockableNode)
				assert.Equal(t, 2, node.futureBlocksCache.Len(), "Future blocks cache should contain 2 blocks")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, restore := captureLogs()
			defer restore()

			node, mockComm := setupTestNodeForHousekeeping(t)

			// Reset mock state
			mockComm.broadcastCalled = false
			mockComm.broadcastBlock = nil

			// Clear future blocks cache
			node.futureBlocksCache = cache.NewRandCache(32)

			// Create test block
			testBlock := tt.setupBlock(node)
			newBlockEvent := &comm.NewBlockEvent{Block: testBlock}

			node.handleNewBlock(newBlockEvent)

			if tt.assertFunc != nil {
				tt.assertFunc(t, map[string]any{
					"node": node,
					"logs": buf.String(),
				})
			}
		})
	}
}

func TestNode_HouseKeeping_FutureTicker(t *testing.T) {
	buf, restore := captureLogs()
	defer restore()

	node, _ := setupTestNodeForHousekeeping(t)

	normalBlock := createTestBlock(node.repo.BestBlockSummary().Header.ID())

	futureBlock := createTestBlock(normalBlock.Header().ID())
	futureBlockID := futureBlock.Header().ID()

	normalBlockEvent := &comm.NewBlockEvent{Block: normalBlock}
	node.handleNewBlock(normalBlockEvent)

	node.futureBlocksCache.Set(futureBlockID, futureBlock)

	assert.True(t, node.futureBlocksCache.Contains(futureBlockID), "Future blocks cache should contain the future block before processing")

	node.handleFutureBlocks()

	assert.False(t, node.futureBlocksCache.Contains(futureBlockID), "Future blocks cache should not contain the future block after processing")
	assert.Contains(t, buf.String(), "future block consumed", "Logs should indicate future block was consumed")
	assert.Contains(t, buf.String(), futureBlockID.String(), "Logs should contain the future block ID")
	assert.Contains(t, buf.String(), "imported blocks", "Logs should indicate blocks were imported")
}

func TestNode_HouseKeeping_ConnectivityTicker(t *testing.T) {
	cs := new(ConnectivityState)

	cs.Check(5)
	assert.Equal(t, ConnectivityState(0), *cs, "cs should be 0 when peers are connected")

	cs.Check(0)
	assert.Equal(t, ConnectivityState(1), *cs, "cs should be 1 after reaching threshold and checking clock offset")
}

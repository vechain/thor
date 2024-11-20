package node

import (
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vrf"
	"testing"
)

func BenchmarkProcessBlock(b *testing.B) {
	// Initialize the test chain and dependencies
	thorChain, err := testchain.NewIntegrationTestChain()
	require.NoError(b, err)

	privateKey := genesis.DevAccounts()[0].PrivateKey
	require.NoError(b, err)
	master := &Master{
		PrivateKey: privateKey,
	}
	repo := thorChain.Repo()
	stater := thorChain.Stater()
	engine, err := bft.NewEngine(thorChain.Repo(), thorChain.Database(), thorChain.GetForkConfig(), master.Address())
	require.NoError(b, err)

	forkConfig := thor.NoFork
	forkConfig.VIP191 = 1
	forkConfig.BLOCKLIST = 0
	forkConfig.VIP214 = 2

	node := New(
		master,
		repo,
		engine,
		stater,
		nil, // logDB (optional)
		nil,
		"",      // txStashPath
		nil,     // communicator
		1000000, // targetGasLimit
		true,    // skipLogs
		forkConfig,
	)

	// Start with the Genesis block
	genesisBlock := thorChain.GenesisBlock()
	previousBlock := genesisBlock
	stats := &blockStats{}

	// Helper function to create sequential blocks
	createMockBlock := func(parentBlock *block.Block, txCount int, gasUsed uint64) *block.Block {
		builder := &block.Builder{}
		builder.ParentID(parentBlock.Header().ID())
		builder.Timestamp(parentBlock.Header().Timestamp() + 100)
		builder.GasLimit(10_000_000)
		builder.TotalScore(parentBlock.Header().TotalScore() + 1)
		builder.GasUsed(gasUsed)
		builder.TransactionFeatures(1)
		builder.ReceiptsRoot(tx.Receipts{}.RootHash())

		// sending money to random account expands the trie leaves
		// ( ooc the trie accounts leaves have the contract storage inside of them)
		root := trie.Root{
			Hash: parentBlock.Header().StateRoot(),
			Ver: trie.Version{
				Major: parentBlock.Header().Number(),
				Minor: 0,
			},
		}
		stage, err := thorChain.Stater().NewState(root).Stage(trie.Version{Major: parentBlock.Header().Number(), Minor: 0})
		require.NoError(b, err)

		stateRoot := stage.Hash()
		builder.StateRoot(stateRoot)

		blk, err := signWithKey(parentBlock, builder, privateKey, forkConfig)
		require.NoError(b, err)
		return blk
	}

	// pre-alloc blocks
	var blocks []*block.Block
	for i := 0; i < 10_000; i++ {
		mockBlock := createMockBlock(previousBlock, 100, 0) // 100 transactions, 500,000 gas used
		blocks = append(blocks, mockBlock)
		previousBlock = mockBlock
	}

	// Measure memory usage
	b.ReportAllocs()

	// Benchmark execution
	b.ResetTimer()
	for _, blk := range blocks {
		_, err := node.processBlock(blk, stats)
		if err != nil {
			b.Fatalf("processBlock failed: %v", err)
		}
	}
}

func signWithKey(parentBlk *block.Block, builder *block.Builder, pk *ecdsa.PrivateKey, forkConfig thor.ForkConfig) (*block.Block, error) {
	h := builder.Build().Header()

	if h.Number() >= forkConfig.VIP214 {
		var alpha []byte
		if h.Number() == forkConfig.VIP214 {
			alpha = parentBlk.Header().StateRoot().Bytes()
		} else {
			beta, err := parentBlk.Header().Beta()
			if err != nil {
				return nil, err
			}
			alpha = beta
		}
		_, proof, err := vrf.Prove(pk, alpha)
		if err != nil {
			return nil, err
		}

		blk := builder.Alpha(alpha).Build()

		ec, err := crypto.Sign(blk.Header().SigningHash().Bytes(), pk)
		if err != nil {
			return nil, err
		}

		sig, err := block.NewComplexSignature(ec, proof)
		if err != nil {
			return nil, err
		}
		return blk.WithSignature(sig), nil
	} else {
		blk := builder.Build()

		sig, err := crypto.Sign(blk.Header().SigningHash().Bytes(), pk)
		if err != nil {
			return nil, err
		}

		return blk.WithSignature(sig), nil
	}
}

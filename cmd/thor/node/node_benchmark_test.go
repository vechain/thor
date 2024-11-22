package node

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/runtime"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/test/datagen"
	"github.com/vechain/thor/v2/test/testchain"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thor/v2/vrf"
	"github.com/vechain/thor/v2/xenv"
	"math"
	"math/big"
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
		"",         // txStashPath
		nil,        // communicator
		10_000_000, // targetGasLimit
		true,       // skipLogs
		forkConfig,
	)

	// Start with the Genesis block
	genesisBlock := thorChain.GenesisBlock()
	previousBlock := genesisBlock
	stats := &blockStats{}

	// Helper function to create sequential blocks
	createMockBlock := func(parentBlock *block.Block, currentChain *testchain.Chain) *block.Block {
		executionChain := currentChain.Repo().NewBestChain()
		fmt.Println(currentChain.Repo().BestBlockSummary().Header.ID().String())
		executionStater := currentChain.Stater()
		executionState := executionStater.NewState(trie.Root{
			Hash: previousBlock.Header().StateRoot(),
			Ver: trie.Version{
				Major: previousBlock.Header().Number(),
				Minor: 0,
			},
		})
		execEngine := newExecutionEngine(currentChain, parentBlock, executionChain, executionState)

		builder := new(block.Builder)

		var receipts tx.Receipts
		gasUsed := uint64(0)
		for gasUsed < 9_500_000 {
			toAddr := datagen.RandAddress()
			cla := tx.NewClause(&toAddr).WithValue(big.NewInt(10000))
			transaction := new(tx.Builder).
				ChainTag(currentChain.Repo().ChainTag()).
				GasPriceCoef(1).
				Expiration(math.MaxUint32 - 1).
				Gas(21_000).
				Nonce(uint64(datagen.RandInt())).
				Clause(cla).
				BlockRef(tx.NewBlockRef(0)).
				Build()

			sig, err := crypto.Sign(transaction.SigningHash().Bytes(), genesis.DevAccounts()[0].PrivateKey)
			require.NoError(b, err)
			transaction = transaction.WithSignature(sig)
			builder.Transaction(transaction)

			// calculate the receipt hash root
			receipt, err := execEngine.rt.ExecuteTransaction(transaction)
			require.NoError(b, err)
			receipts = append(receipts, receipt)

			gasUsed += 21_000 // Gas per transaction
		}

		prevblockRoot := trie.Root{
			Hash: previousBlock.Header().StateRoot(),
			Ver: trie.Version{
				Major: previousBlock.Header().Number(),
				Minor: 0,
			},
		}
		fmt.Println("PrevBlock StateRoot: ", prevblockRoot.Hash.String())

		stage, err := executionState.Stage(trie.Version{Major: parentBlock.Header().Number() + 1, Minor: 0})
		require.NoError(b, err)
		stateRoot, err := stage.Commit()
		fmt.Println("Mocked StateRoot: ", stateRoot.String())
		require.NoError(b, err)

		builder.ParentID(parentBlock.Header().ID()).
			Timestamp(parentBlock.Header().Timestamp() + 100).
			GasLimit(10_000_000).
			TotalScore(parentBlock.Header().TotalScore() + 1).
			TransactionFeatures(1).
			GasUsed(gasUsed).
			ReceiptsRoot(receipts.RootHash()).
			StateRoot(stateRoot)

		blk, err := signWithKeyAndBuild(parentBlock, builder, privateKey, forkConfig)
		require.NoError(b, err)

		require.NoError(b, currentChain.Repo().AddBlock(blk, receipts, 0, true))
		return blk
	}

	// pre-alloc blocks
	var blocks []*block.Block
	currentChain, err := testchain.NewIntegrationTestChain()
	require.NoError(b, err)

	for i := 0; i < 10; i++ {
		mockBlock := createMockBlock(previousBlock, currentChain)
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

func signWithKeyAndBuild(parentBlk *block.Block, builder *block.Builder, pk *ecdsa.PrivateKey, forkConfig thor.ForkConfig) (*block.Block, error) {
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

type executionEngined struct {
	rt *runtime.Runtime
}

func newExecutionEngine(
	thorChain *testchain.Chain,
	parentBlk *block.Block,
	executionChain *chain.Chain,
	executionState *state.State,
) *executionEngined {
	signer, err := parentBlk.Header().Signer()
	if err != nil {
		panic(err)
	}
	return &executionEngined{
		rt: runtime.New(
			executionChain,
			executionState,
			&xenv.BlockContext{
				Beneficiary: parentBlk.Header().Beneficiary(),
				Signer:      signer,
				Number:      parentBlk.Header().Number(),
				Time:        parentBlk.Header().Timestamp(),
				GasLimit:    parentBlk.Header().GasLimit(),
				TotalScore:  parentBlk.Header().TotalScore(),
			},
			thorChain.GetForkConfig()),
	}
}

//func BuildBlock(stater *state.Stater) (blk *block.Block, events tx.Events, transfers tx.Transfers, err error) {
//	state := stater.NewState(trie.Root{})
//
//	for _, proc := range b.stateProcs {
//		if err := proc(state); err != nil {
//			return nil, nil, nil, errors.Wrap(err, "state process")
//		}
//	}
//
//	rt := runtime.New(nil, state, &xenv.BlockContext{
//		Time:     b.timestamp,
//		GasLimit: b.gasLimit,
//	}, b.forkConfig)
//
//	for _, call := range b.calls {
//		exec, _ := rt.PrepareClause(call.clause, 0, math.MaxUint64, &xenv.TransactionContext{
//			Origin: call.caller,
//		})
//		out, _, err := exec()
//		if err != nil {
//			return nil, nil, nil, errors.Wrap(err, "call")
//		}
//		if out.VMErr != nil {
//			return nil, nil, nil, errors.Wrap(out.VMErr, "vm")
//		}
//		events = append(events, out.Events...)
//		transfers = append(transfers, out.Transfers...)
//	}
//
//	stage, err := state.Stage(trie.Version{})
//	if err != nil {
//		return nil, nil, nil, errors.Wrap(err, "stage")
//	}
//	stateRoot, err := stage.Commit()
//	if err != nil {
//		return nil, nil, nil, errors.Wrap(err, "commit state")
//	}
//
//	parentID := thor.Bytes32{0xff, 0xff, 0xff, 0xff} //so, genesis number is 0
//	copy(parentID[4:], b.extraData[:])
//
//	return new(block.Builder).
//		ParentID(parentID).
//		Timestamp(b.timestamp).
//		GasLimit(b.gasLimit).
//		StateRoot(stateRoot).
//		ReceiptsRoot(tx.Transactions(nil).RootHash()).
//		Build(), events, transfers, nil
//}

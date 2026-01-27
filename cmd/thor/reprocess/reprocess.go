package reprocess

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/kv"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const (
	txFlag = byte(0) // flag byte of the key for saving tx blob
)

// ReprocessChainFromSnapshot reprocesses all blocks from a snapshot data directory
func ReprocessChainFromSnapshot(
	snapshotDataDir string,
	targetDB *muxdb.MuxDB,
	targetLogDB *logdb.LogDB,
) error {
	log.Info("Opening snapshot database", "dir", snapshotDataDir)

	// Open source database from snapshot
	snapshotDBPath := filepath.Join(snapshotDataDir, "main.db")
	if _, err := os.Stat(snapshotDBPath); err != nil {
		return errors.Wrapf(err, "snapshot database not found at %s", snapshotDBPath)
	}

	// Open snapshot database
	opts := muxdb.Options{
		TrieNodeCacheSizeMB:        512,
		TrieCachedNodeTTL:          30,
		TrieDedupedPartitionFactor: math.MaxUint32,
		TrieWillCleanHistory:       false, // Don't prune when reading
		OpenFilesCacheCapacity:     5000,
		ReadCacheMB:                256,
		WriteBufferMB:              128,
		TrieHistPartitionFactor:    524288, // Large for archive nodes
	}

	snapshotDB, err := muxdb.Open(snapshotDBPath, &opts)
	if err != nil {
		return errors.Wrap(err, "open snapshot database")
	}
	defer snapshotDB.Close()

	// Get genesis from snapshot
	genesisBlock, forkConfig, genesisGene, err := detectGenesisFromSnapshot(snapshotDB, snapshotDataDir)
	if err != nil {
		return errors.Wrap(err, "detect genesis from snapshot")
	}

	log.Info("Detected genesis", "id", genesisBlock.Header().ID(), "forkConfig", forkConfig)

	// Check if target repository already exists
	targetRepo, err := chain.NewRepository(targetDB, genesisBlock)
	if err != nil {
		return errors.Wrap(err, "open target repository")
	}

	// Check current progress in target database
	targetBestSummary := targetRepo.BestBlockSummary()
	targetBestBlockNum := targetBestSummary.Header.Number()

	// Build genesis state only if target is empty (genesis not yet processed)
	if targetBestBlockNum == 0 {
		log.Info("Initializing genesis state in target database")
		targetStater := state.NewStater(targetDB)
		builtGenesisBlock, genesisEvents, genesisTransfers, err := genesisGene.Build(targetStater)
		if err != nil {
			return errors.Wrap(err, "build genesis state in target database")
		}

		// Verify the built genesis matches the snapshot genesis
		if builtGenesisBlock.Header().ID() != genesisBlock.Header().ID() {
			return errors.Errorf("genesis ID mismatch: built %v, snapshot %v",
				builtGenesisBlock.Header().ID(), genesisBlock.Header().ID())
		}

		// Re-initialize repository after genesis is built
		targetRepo, err = chain.NewRepository(targetDB, builtGenesisBlock)
		if err != nil {
			return errors.Wrap(err, "re-initialize target repository")
		}

		// Write genesis logs to target log DB
		logWriter := targetLogDB.NewWriter()
		if err := logWriter.Write(builtGenesisBlock, tx.Receipts{{
			Outputs: []*tx.Output{
				{Events: genesisEvents, Transfers: genesisTransfers},
			},
		}}); err != nil {
			return errors.Wrap(err, "write genesis logs")
		}
		if err := logWriter.Commit(); err != nil {
			return errors.Wrap(err, "commit genesis logs")
		}
	} else {
		log.Info("Resuming reprocessing", "currentBlock", targetBestBlockNum)
	}

	// Open snapshot repository
	snapshotRepo, err := chain.NewRepository(snapshotDB, genesisBlock)
	if err != nil {
		return errors.Wrap(err, "open snapshot repository")
	}

	// Get best block from snapshot
	bestSummary := snapshotRepo.BestBlockSummary()
	bestBlockNum := bestSummary.Header.Number()
	log.Info("Snapshot best block", "number", bestBlockNum, "id", bestSummary.Header.ID())

	// Check if already complete
	if targetBestBlockNum >= bestBlockNum {
		log.Info("Reprocessing already complete",
			"targetBlock", targetBestBlockNum,
			"snapshotBlock", bestBlockNum)
		return nil
	}

	// Initialize state manager and consensus
	stater := state.NewStater(targetDB)
	cons := consensus.New(targetRepo, stater, forkConfig)

	// Get best chain from snapshot
	snapshotBestChain := snapshotRepo.NewBestChain()

	// Start from next block after target's best block
	startBlockNum := targetBestBlockNum + 1
	log.Info("Starting reprocessing",
		"startBlock", startBlockNum,
		"endBlock", bestBlockNum,
		"totalBlocks", bestBlockNum-targetBestBlockNum)

	for blockNum := startBlockNum; blockNum <= bestBlockNum; blockNum++ {
		// Get block from snapshot
		snapshotBlock, err := snapshotBestChain.GetBlock(blockNum)
		if err != nil {
			return errors.Wrapf(err, "get block %d from snapshot", blockNum)
		}

		// Get parent summary from target (already processed)
		parentSummary, err := targetRepo.GetBlockSummary(snapshotBlock.Header().ParentID())
		if err != nil {
			return errors.Wrapf(err, "get parent summary for block %d", blockNum)
		}

		// Process block (validate and execute)
		stage, receipts, err := cons.Process(
			parentSummary,
			snapshotBlock,
			snapshotBlock.Header().Timestamp(),
			0, // conflicts
		)
		if err != nil {
			return errors.Wrapf(err, "failed to process block %d", blockNum)
		}

		// Commit state
		if _, err := stage.Commit(); err != nil {
			return errors.Wrapf(err, "commit state for block %d", blockNum)
		}

		// Add block to target repository
		if err := targetRepo.AddBlock(snapshotBlock, receipts, 0, true); err != nil {
			return errors.Wrapf(err, "add block %d to target repository", blockNum)
		}

		// Write logs to target log DB
		logWriter := targetLogDB.NewWriter()
		if err := logWriter.Write(snapshotBlock, receipts); err != nil {
			return errors.Wrapf(err, "write logs for block %d", blockNum)
		}
		if err := logWriter.Commit(); err != nil {
			return errors.Wrapf(err, "commit logs for block %d", blockNum)
		}

		// Log progress
		if blockNum%1000 == 0 || blockNum == bestBlockNum {
			log.Info("Reprocessing progress",
				"block", blockNum,
				"total", bestBlockNum,
				"percent", fmt.Sprintf("%.2f%%", float64(blockNum)/float64(bestBlockNum)*100))
		}
	}

	log.Info("Reprocessing completed successfully", "totalBlocks", bestBlockNum+1)
	return nil
}

// detectGenesisFromSnapshot detects genesis block, fork config, and genesis builder from snapshot
func detectGenesisFromSnapshot(db *muxdb.MuxDB, dataDir string) (*block.Block, *thor.ForkConfig, *genesis.Genesis, error) {
	// First, try to read the genesis ID from the database directly
	propStore := db.NewStore("chain.props")
	bestBlockIDKey := []byte("best-block-id")

	bestBlockIDBytes, err := propStore.Get(bestBlockIDKey)
	if err != nil {
		if propStore.IsNotFound(err) {
			return nil, nil, nil, errors.New("snapshot database appears empty - no best block found")
		}
		return nil, nil, nil, errors.Wrap(err, "read best block ID")
	}

	bestBlockID := thor.BytesToBytes32(bestBlockIDBytes)

	// Read the best block summary to get the index root
	hdrStore := db.NewStore("chain.hdr")
	bestSummary, err := loadBlockSummary(hdrStore, bestBlockID)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "load best block summary")
	}

	// Get the index trie to read block 0 (genesis)
	indexTrie := db.NewTrie(muxdb.IndexTrieName, bestSummary.IndexRoot())
	var genesisKey [4]byte
	binary.BigEndian.PutUint32(genesisKey[:], 0)

	genesisIDBytes, _, err := indexTrie.Get(genesisKey[:])
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "get genesis block ID from index trie")
	}

	if len(genesisIDBytes) == 0 {
		return nil, nil, nil, errors.New("genesis block not found in index trie")
	}

	genesisID := thor.BytesToBytes32(genesisIDBytes)

	log.Info("Found genesis ID in snapshot", "id", genesisID)

	// Now try to match with known genesis blocks
	for _, geneInfo := range []struct {
		name string
		fn   func() (*genesis.Genesis, *thor.ForkConfig)
	}{
		{"mainnet", func() (*genesis.Genesis, *thor.ForkConfig) {
			g := genesis.NewMainnet()
			return g, thor.GetForkConfig(g.ID())
		}},
		{"testnet", func() (*genesis.Genesis, *thor.ForkConfig) {
			g := genesis.NewTestnet()
			return g, thor.GetForkConfig(g.ID())
		}},
	} {
		gene, forkConfig := geneInfo.fn()
		if gene.ID() == genesisID {
			// Build the genesis block to get the actual block object
			stater := state.NewStater(db)
			genesisBlock, _, _, err := gene.Build(stater)
			if err != nil {
				log.Warn("Failed to build genesis block", "network", geneInfo.name, "err", err)
				continue
			}

			log.Info("Matched genesis", "network", geneInfo.name, "id", genesisID)
			return genesisBlock, forkConfig, gene, nil
		}
	}

	// If no match found, we can't build the genesis state
	return nil, nil, nil, errors.Errorf("unknown genesis ID %v - cannot build genesis state. Please use a known network (mainnet/testnet)", genesisID)
}

// Helper function to load block summary (similar to chain/repository.go)
func loadBlockSummary(r kv.Getter, id thor.Bytes32) (*chain.BlockSummary, error) {
	// This should match the implementation in chain/persist.go
	// We need to decode the RLP-encoded block summary
	data, err := r.Get(id.Bytes())
	if err != nil {
		return nil, err
	}

	var summary chain.BlockSummary
	if err := rlp.DecodeBytes(data, &summary); err != nil {
		return nil, err
	}

	return &summary, nil
}

// Helper function to get block transactions
func getBlockTransactions(bodyStore kv.Store, summary *chain.BlockSummary) (tx.Transactions, error) {
	if len(summary.Txs) == 0 {
		return nil, nil
	}

	txs := make(tx.Transactions, len(summary.Txs))
	var key []byte
	for i := range summary.Txs {
		key = appendTxKey(key[:0], summary.Header.Number(), summary.Conflicts, uint64(i), txFlag)
		txData, err := bodyStore.Get(key)
		if err != nil {
			return nil, errors.Wrapf(err, "get transaction %d", i)
		}

		var transaction tx.Transaction
		if err := rlp.DecodeBytes(txData, &transaction); err != nil {
			return nil, errors.Wrapf(err, "decode transaction %d", i)
		}
		txs[i] = &transaction
	}

	return txs, nil
}

// Helper to append transaction key (from chain/persist.go)
func appendTxKey(buf []byte, blockNum, blockConflicts uint32, index uint64, flag byte) []byte {
	buf = binary.BigEndian.AppendUint32(buf, blockNum)
	buf = binary.AppendUvarint(buf, uint64(blockConflicts))
	buf = append(buf, flag)
	return binary.AppendUvarint(buf, index)
}

// ReprocessChainFromSnapshotWithGenesis is like ReprocessChainFromSnapshot but with explicit genesis
func ReprocessChainFromSnapshotWithGenesis(
	snapshotDataDir string,
	targetDB *muxdb.MuxDB,
	targetLogDB *logdb.LogDB,
	genesisGene *genesis.Genesis,
	forkConfig *thor.ForkConfig,
) error {
	log.Info("Opening snapshot database", "dir", snapshotDataDir)

	// Open source database from snapshot
	snapshotDBPath := filepath.Join(snapshotDataDir, "main.db")
	if _, err := os.Stat(snapshotDBPath); err != nil {
		return errors.Wrapf(err, "snapshot database not found at %s", snapshotDBPath)
	}

	opts := muxdb.Options{
		TrieNodeCacheSizeMB:        512,
		TrieCachedNodeTTL:          30,
		TrieDedupedPartitionFactor: math.MaxUint32,
		TrieWillCleanHistory:       false,
		OpenFilesCacheCapacity:     5000,
		ReadCacheMB:                256,
		WriteBufferMB:              128,
		TrieHistPartitionFactor:    524288,
	}

	snapshotDB, err := muxdb.Open(snapshotDBPath, &opts)
	if err != nil {
		return errors.Wrap(err, "open snapshot database")
	}
	defer snapshotDB.Close()

	// Build genesis block to get the block object
	targetStater := state.NewStater(targetDB)
	genesisBlock, genesisEvents, genesisTransfers, err := genesisGene.Build(targetStater)
	if err != nil {
		return errors.Wrap(err, "build genesis state in target database")
	}

	// Check if target repository already exists
	targetRepo, err := chain.NewRepository(targetDB, genesisBlock)
	if err != nil {
		return errors.Wrap(err, "open target repository")
	}

	// Check current progress in target database
	targetBestSummary := targetRepo.BestBlockSummary()
	targetBestBlockNum := targetBestSummary.Header.Number()

	// Build genesis state only if target is empty (genesis not yet processed)
	if targetBestBlockNum == 0 {
		log.Info("Initializing genesis state in target database")

		// Re-initialize repository after genesis is built
		targetRepo, err = chain.NewRepository(targetDB, genesisBlock)
		if err != nil {
			return errors.Wrap(err, "re-initialize target repository")
		}

		// Write genesis logs
		logWriter := targetLogDB.NewWriter()
		if err := logWriter.Write(genesisBlock, tx.Receipts{{
			Outputs: []*tx.Output{
				{Events: genesisEvents, Transfers: genesisTransfers},
			},
		}}); err != nil {
			return errors.Wrap(err, "write genesis logs")
		}
		if err := logWriter.Commit(); err != nil {
			return errors.Wrap(err, "commit genesis logs")
		}
	} else {
		log.Info("Resuming reprocessing", "currentBlock", targetBestBlockNum)
	}

	// Open snapshot repository
	snapshotRepo, err := chain.NewRepository(snapshotDB, genesisBlock)
	if err != nil {
		return errors.Wrap(err, "open snapshot repository")
	}

	// Get best block from snapshot
	bestSummary := snapshotRepo.BestBlockSummary()
	bestBlockNum := bestSummary.Header.Number()
	log.Info("Snapshot best block", "number", bestBlockNum, "id", bestSummary.Header.ID())

	// Check if already complete
	if targetBestBlockNum >= bestBlockNum {
		log.Info("Reprocessing already complete",
			"targetBlock", targetBestBlockNum,
			"snapshotBlock", bestBlockNum)
		return nil
	}

	stater := state.NewStater(targetDB)
	cons := consensus.New(targetRepo, stater, forkConfig)

	snapshotBestChain := snapshotRepo.NewBestChain()

	// Start from next block after target's best block
	startBlockNum := targetBestBlockNum + 1
	log.Info("Starting reprocessing",
		"startBlock", startBlockNum,
		"endBlock", bestBlockNum,
		"totalBlocks", bestBlockNum-targetBestBlockNum)

	for blockNum := startBlockNum; blockNum <= bestBlockNum; blockNum++ {
		snapshotBlock, err := snapshotBestChain.GetBlock(blockNum)
		if err != nil {
			return errors.Wrapf(err, "get block %d from snapshot", blockNum)
		}

		parentSummary, err := targetRepo.GetBlockSummary(snapshotBlock.Header().ParentID())
		if err != nil {
			return errors.Wrapf(err, "get parent summary for block %d", blockNum)
		}

		stage, receipts, err := cons.Process(
			parentSummary,
			snapshotBlock,
			snapshotBlock.Header().Timestamp(),
			0,
		)
		if err != nil {
			return errors.Wrapf(err, "failed to process block %d", blockNum)
		}

		if _, err := stage.Commit(); err != nil {
			return errors.Wrapf(err, "commit state for block %d", blockNum)
		}

		if err := targetRepo.AddBlock(snapshotBlock, receipts, 0, true); err != nil {
			return errors.Wrapf(err, "add block %d to target repository", blockNum)
		}

		logWriter := targetLogDB.NewWriter()
		if err := logWriter.Write(snapshotBlock, receipts); err != nil {
			return errors.Wrapf(err, "write logs for block %d", blockNum)
		}
		if err := logWriter.Commit(); err != nil {
			return errors.Wrapf(err, "commit logs for block %d", blockNum)
		}

		if blockNum%1000 == 0 || blockNum == bestBlockNum {
			log.Info("Reprocessing progress",
				"block", blockNum,
				"total", bestBlockNum,
				"percent", fmt.Sprintf("%.2f%%", float64(blockNum)/float64(bestBlockNum)*100))
		}
	}

	log.Info("Reprocessing completed successfully", "totalBlocks", bestBlockNum+1)
	return nil
}

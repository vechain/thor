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
	txFlag       = byte(0)
	logBatchSize = 1000
)

// ReprocessChainFromSnapshot reprocesses all blocks from a snapshot data directory
func ReprocessChainFromSnapshot(
	snapshotDataDir string,
	targetDB *muxdb.MuxDB,
	targetLogDB *logdb.LogDB,
	skipLogs bool,
) error {
	log.Info("Opening snapshot database", "dir", snapshotDataDir)

	snapshotDBPath := filepath.Join(snapshotDataDir, "main.db")
	if _, err := os.Stat(snapshotDBPath); err != nil {
		return errors.Wrapf(err, "snapshot database not found at %s", snapshotDBPath)
	}

	opts := muxdb.Options{
		TrieNodeCacheSizeMB:        1024,
		TrieCachedNodeTTL:          30,
		TrieDedupedPartitionFactor: math.MaxUint32,
		TrieWillCleanHistory:       false,
		OpenFilesCacheCapacity:     10000,
		ReadCacheMB:                512,
		WriteBufferMB:              128,
		TrieHistPartitionFactor:    524288,
	}

	snapshotDB, err := muxdb.Open(snapshotDBPath, &opts)
	if err != nil {
		return errors.Wrap(err, "open snapshot database")
	}
	defer snapshotDB.Close()

	genesisBlock, forkConfig, genesisGene, err := detectGenesisFromSnapshot(snapshotDB, snapshotDataDir)
	if err != nil {
		return errors.Wrap(err, "detect genesis from snapshot")
	}

	log.Info("Detected genesis", "id", genesisBlock.Header().ID(), "forkConfig", forkConfig)

	targetRepo, err := chain.NewRepository(targetDB, genesisBlock)
	if err != nil {
		return errors.Wrap(err, "open target repository")
	}

	targetBestSummary := targetRepo.BestBlockSummary()
	targetBestBlockNum := targetBestSummary.Header.Number()

	if targetBestBlockNum == 0 {
		log.Info("Initializing genesis state in target database")
		targetStater := state.NewStater(targetDB)
		builtGenesisBlock, genesisEvents, genesisTransfers, err := genesisGene.Build(targetStater)
		if err != nil {
			return errors.Wrap(err, "build genesis state in target database")
		}

		if builtGenesisBlock.Header().ID() != genesisBlock.Header().ID() {
			return errors.Errorf("genesis ID mismatch: built %v, snapshot %v",
				builtGenesisBlock.Header().ID(), genesisBlock.Header().ID())
		}

		targetRepo, err = chain.NewRepository(targetDB, builtGenesisBlock)
		if err != nil {
			return errors.Wrap(err, "re-initialize target repository")
		}

		if !skipLogs {
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
		}
	} else {
		log.Info("Resuming reprocessing", "currentBlock", targetBestBlockNum)
		fmt.Printf("[REPROCESS] Resuming from block %d\n", targetBestBlockNum)
	}

	snapshotRepo, err := chain.NewRepository(snapshotDB, genesisBlock)
	if err != nil {
		return errors.Wrap(err, "open snapshot repository")
	}

	bestSummary := snapshotRepo.BestBlockSummary()
	bestBlockNum := bestSummary.Header.Number()
	log.Info("Snapshot best block", "number", bestBlockNum, "id", bestSummary.Header.ID())

	if targetBestBlockNum >= bestBlockNum {
		log.Info("Reprocessing already complete",
			"targetBlock", targetBestBlockNum,
			"snapshotBlock", bestBlockNum)
		fmt.Printf("[REPROCESS] Already complete: target at block %d, snapshot at block %d\n",
			targetBestBlockNum, bestBlockNum)
		return nil
	}

	stater := state.NewStater(targetDB)
	cons := consensus.New(targetRepo, stater, forkConfig)

	snapshotBestChain := snapshotRepo.NewBestChain()

	startBlockNum := targetBestBlockNum + 1
	totalBlocksToProcess := bestBlockNum - targetBestBlockNum
	log.Info("Starting reprocessing",
		"startBlock", startBlockNum,
		"endBlock", bestBlockNum,
		"totalBlocks", totalBlocksToProcess)
	fmt.Printf("[REPROCESS] Starting from block %d to %d (%d blocks to process)\n",
		startBlockNum, bestBlockNum, totalBlocksToProcess)

	var logWriter *logdb.Writer
	if !skipLogs {
		logWriter = targetLogDB.NewWriter()
	}

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

		if !skipLogs {
			if err := logWriter.Write(snapshotBlock, receipts); err != nil {
				return errors.Wrapf(err, "write logs for block %d", blockNum)
			}

			blocksProcessed := blockNum - startBlockNum + 1
			shouldCommitLogs := blocksProcessed%logBatchSize == 0 || blockNum == bestBlockNum
			if shouldCommitLogs {
				if err := logWriter.Commit(); err != nil {
					return errors.Wrapf(err, "commit logs at block %d", blockNum)
				}
			}
		}

		if blockNum%1000 == 0 || blockNum%100000 == 0 || blockNum == bestBlockNum {
			percent := float64(blockNum) / float64(bestBlockNum) * 100
			remaining := bestBlockNum - blockNum
			log.Info("Reprocessing progress",
				"block", blockNum,
				"total", bestBlockNum,
				"percent", fmt.Sprintf("%.2f%%", percent),
				"remaining", remaining)
			if blockNum%100000 == 0 || blockNum == bestBlockNum {
				fmt.Printf("[REPROCESS] Block %d / %d (%.2f%%) - %d blocks remaining\n",
					blockNum, bestBlockNum, percent, remaining)
			}
		}
	}

	if !skipLogs {
		if err := logWriter.Commit(); err != nil {
			return errors.Wrap(err, "final commit logs")
		}
	}

	log.Info("Reprocessing completed successfully", "totalBlocks", bestBlockNum+1)
	fmt.Printf("[REPROCESS] Completed successfully: processed %d blocks\n", totalBlocksToProcess)
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

	hdrStore := db.NewStore("chain.hdr")
	bestSummary, err := loadBlockSummary(hdrStore, bestBlockID)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "load best block summary")
	}

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

	return nil, nil, nil, errors.Errorf("unknown genesis ID %v - cannot build genesis state. Please use a known network (mainnet/testnet)", genesisID)
}

func loadBlockSummary(r kv.Getter, id thor.Bytes32) (*chain.BlockSummary, error) {
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

// ReprocessChainFromSnapshotWithGenesis is like ReprocessChainFromSnapshot but with explicit genesis
func ReprocessChainFromSnapshotWithGenesis(
	snapshotDataDir string,
	targetDB *muxdb.MuxDB,
	targetLogDB *logdb.LogDB,
	genesisGene *genesis.Genesis,
	forkConfig *thor.ForkConfig,
	skipLogs bool,
) error {
	log.Info("Opening snapshot database", "dir", snapshotDataDir)

	snapshotDBPath := filepath.Join(snapshotDataDir, "main.db")
	if _, err := os.Stat(snapshotDBPath); err != nil {
		return errors.Wrapf(err, "snapshot database not found at %s", snapshotDBPath)
	}

	opts := muxdb.Options{
		TrieNodeCacheSizeMB:        1024,
		TrieCachedNodeTTL:          30,
		TrieDedupedPartitionFactor: math.MaxUint32,
		TrieWillCleanHistory:       false,
		OpenFilesCacheCapacity:     10000,
		ReadCacheMB:                512,
		WriteBufferMB:              128,
		TrieHistPartitionFactor:    524288,
	}

	snapshotDB, err := muxdb.Open(snapshotDBPath, &opts)
	if err != nil {
		return errors.Wrap(err, "open snapshot database")
	}
	defer snapshotDB.Close()

	targetStater := state.NewStater(targetDB)
	genesisBlock, genesisEvents, genesisTransfers, err := genesisGene.Build(targetStater)
	if err != nil {
		return errors.Wrap(err, "build genesis state in target database")
	}

	targetRepo, err := chain.NewRepository(targetDB, genesisBlock)
	if err != nil {
		return errors.Wrap(err, "open target repository")
	}

	targetBestSummary := targetRepo.BestBlockSummary()
	targetBestBlockNum := targetBestSummary.Header.Number()

	if targetBestBlockNum == 0 {
		log.Info("Initializing genesis state in target database")

		targetRepo, err = chain.NewRepository(targetDB, genesisBlock)
		if err != nil {
			return errors.Wrap(err, "re-initialize target repository")
		}

		if !skipLogs {
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
		}
	} else {
		log.Info("Resuming reprocessing", "currentBlock", targetBestBlockNum)
		fmt.Printf("[REPROCESS] Resuming from block %d\n", targetBestBlockNum)
	}

	snapshotRepo, err := chain.NewRepository(snapshotDB, genesisBlock)
	if err != nil {
		return errors.Wrap(err, "open snapshot repository")
	}

	bestSummary := snapshotRepo.BestBlockSummary()
	bestBlockNum := bestSummary.Header.Number()
	log.Info("Snapshot best block", "number", bestBlockNum, "id", bestSummary.Header.ID())

	if targetBestBlockNum >= bestBlockNum {
		log.Info("Reprocessing already complete",
			"targetBlock", targetBestBlockNum,
			"snapshotBlock", bestBlockNum)
		fmt.Printf("[REPROCESS] Already complete: target at block %d, snapshot at block %d\n",
			targetBestBlockNum, bestBlockNum)
		return nil
	}

	stater := state.NewStater(targetDB)
	cons := consensus.New(targetRepo, stater, forkConfig)

	snapshotBestChain := snapshotRepo.NewBestChain()

	startBlockNum := targetBestBlockNum + 1
	totalBlocksToProcess := bestBlockNum - targetBestBlockNum
	log.Info("Starting reprocessing",
		"startBlock", startBlockNum,
		"endBlock", bestBlockNum,
		"totalBlocks", totalBlocksToProcess)
	fmt.Printf("[REPROCESS] Starting from block %d to %d (%d blocks to process)\n",
		startBlockNum, bestBlockNum, totalBlocksToProcess)

	var logWriter *logdb.Writer
	if !skipLogs {
		logWriter = targetLogDB.NewWriter()
	}

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

		if !skipLogs {
			if err := logWriter.Write(snapshotBlock, receipts); err != nil {
				return errors.Wrapf(err, "write logs for block %d", blockNum)
			}

			blocksProcessed := blockNum - startBlockNum + 1
			shouldCommitLogs := blocksProcessed%logBatchSize == 0 || blockNum == bestBlockNum
			if shouldCommitLogs {
				if err := logWriter.Commit(); err != nil {
					return errors.Wrapf(err, "commit logs at block %d", blockNum)
				}
			}
		}

		if blockNum%1000 == 0 || blockNum%100000 == 0 || blockNum == bestBlockNum {
			percent := float64(blockNum) / float64(bestBlockNum) * 100
			remaining := bestBlockNum - blockNum
			log.Info("Reprocessing progress",
				"block", blockNum,
				"total", bestBlockNum,
				"percent", fmt.Sprintf("%.2f%%", percent),
				"remaining", remaining)
			if blockNum%100000 == 0 || blockNum == bestBlockNum {
				fmt.Printf("[REPROCESS] Block %d / %d (%.2f%%) - %d blocks remaining\n",
					blockNum, bestBlockNum, percent, remaining)
			}
		}
	}

	if !skipLogs {
		if err := logWriter.Commit(); err != nil {
			return errors.Wrap(err, "final commit logs")
		}
	}

	log.Info("Reprocessing completed successfully", "totalBlocks", bestBlockNum+1)
	fmt.Printf("[REPROCESS] Completed successfully: processed %d blocks\n", totalBlocksToProcess)
	return nil
}

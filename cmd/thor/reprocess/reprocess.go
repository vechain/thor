// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package reprocess

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"

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
	logBatchSize = 1000
)

type asyncLogWriter struct {
	writer    *logdb.Writer
	writeChan chan logWriteRequest
	errChan   chan error
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	lastErr   error
}

type logWriteRequest struct {
	block    *block.Block
	receipts tx.Receipts
	blockNum uint32
	commit   bool
}

type blockResult struct {
	block *block.Block
	err   error
}

type summaryResult struct {
	summary *chain.BlockSummary
	err     error
}

func newAsyncLogWriter(writer *logdb.Writer) *asyncLogWriter {
	ctx, cancel := context.WithCancel(context.Background())
	aw := &asyncLogWriter{
		writer:    writer,
		writeChan: make(chan logWriteRequest, 100), // buffer for smooth operation
		errChan:   make(chan error, 1),
		ctx:       ctx,
		cancel:    cancel,
	}

	aw.wg.Add(1)
	go aw.writeLoop()
	return aw
}

func (aw *asyncLogWriter) writeLoop() {
	defer aw.wg.Done()

	for {
		select {
		case <-aw.ctx.Done():
			return
		case req, ok := <-aw.writeChan:
			if !ok {
				return
			}

			if err := aw.writer.Write(req.block, req.receipts); err != nil {
				aw.mu.Lock()
				aw.lastErr = err
				aw.mu.Unlock()
				select {
				case aw.errChan <- err:
				default:
				}
				continue
			}

			if req.commit {
				if err := aw.writer.Commit(); err != nil {
					aw.mu.Lock()
					aw.lastErr = err
					aw.mu.Unlock()
					select {
					case aw.errChan <- err:
					default:
					}
				}
			}
		}
	}
}

func (aw *asyncLogWriter) Write(block *block.Block, receipts tx.Receipts, blockNum uint32, commit bool) error {
	aw.mu.Lock()
	if aw.lastErr != nil {
		err := aw.lastErr
		aw.mu.Unlock()
		return err
	}
	aw.mu.Unlock()

	select {
	case <-aw.ctx.Done():
		return aw.ctx.Err()
	case aw.writeChan <- logWriteRequest{block: block, receipts: receipts, blockNum: blockNum, commit: commit}:
		return nil
	}
}

func (aw *asyncLogWriter) Close() error {
	close(aw.writeChan)
	aw.wg.Wait()

	if err := aw.writer.Commit(); err != nil {
		return err
	}

	aw.mu.Lock()
	err := aw.lastErr
	aw.mu.Unlock()
	return err
}

// ReprocessChainFromSnapshot reprocesses all blocks from a snapshot data directory
func ReprocessChainFromSnapshot(
	inputDataDir string,
	outputDatDir string,
	outputLogDB *logdb.LogDB,
	skipLogs bool,
) error {
	inputDB, err := getInputDB(inputDataDir)
	if err != nil {
		return errors.Wrap(err, "open input database")
	}
	defer inputDB.Close()

	outputDB, err := getOutputDB(outputDatDir)
	if err != nil {
		return errors.Wrap(err, "open output database")
	}
	defer outputDB.Close()

	genesisBlock, forkConfig, genesisGene, err := detectGenesisFromSnapshot(inputDB)
	if err != nil {
		return errors.Wrap(err, "detect genesis from snapshot")
	}

	log.Info("Detected genesis", "id", genesisBlock.Header().ID(), "forkConfig", forkConfig)

	targetRepo, err := chain.NewRepository(outputDB, genesisBlock)
	if err != nil {
		return errors.Wrap(err, "open target repository")
	}

	targetBestSummary := targetRepo.BestBlockSummary()
	targetBestBlockNum := targetBestSummary.Header.Number()
	logWriter := outputLogDB.NewWriter()
	var asyncWriter *asyncLogWriter
	if !skipLogs {
		asyncWriter = newAsyncLogWriter(logWriter)
		defer func() {
			if asyncWriter != nil {
				if err := asyncWriter.Close(); err != nil {
					log.Error("Failed to close async log writer", "err", err)
				}
			}
		}()
	}

	if targetBestBlockNum == 0 {
		log.Info("Initializing genesis state in target database")
		targetStater := state.NewStater(outputDB)
		builtGenesisBlock, genesisEvents, genesisTransfers, err := genesisGene.Build(targetStater)
		if err != nil {
			return errors.Wrap(err, "build genesis state in target database")
		}

		if builtGenesisBlock.Header().ID() != genesisBlock.Header().ID() {
			return errors.Errorf("genesis ID mismatch: built %v, snapshot %v",
				builtGenesisBlock.Header().ID(), genesisBlock.Header().ID())
		}

		targetRepo, err = chain.NewRepository(outputDB, builtGenesisBlock)
		if err != nil {
			return errors.Wrap(err, "re-initialize target repository")
		}

		if !skipLogs {
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
	}

	snapshotRepo, err := chain.NewRepository(inputDB, genesisBlock)
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
		log.Info("[REPROCESS] Already complete:", "target at block", targetBestBlockNum, "snapshot at block", bestBlockNum)
		return nil
	}

	stater := state.NewStater(outputDB)
	cons := consensus.New(targetRepo, stater, forkConfig)

	snapshotBestChain := snapshotRepo.NewBestChain()

	startBlockNum := targetBestBlockNum + 1
	totalBlocksToProcess := bestBlockNum - targetBestBlockNum
	log.Info("Starting reprocessing",
		"startBlock", startBlockNum,
		"endBlock", bestBlockNum,
		"totalBlocks", totalBlocksToProcess)

	var prefetchedBlock *blockResult
	if startBlockNum <= bestBlockNum {
		prefetchedBlock = &blockResult{}
		go func(num uint32) {
			blk, err := snapshotBestChain.GetBlock(num)
			prefetchedBlock.block = blk
			prefetchedBlock.err = err
		}(startBlockNum)
	}

	for blockNum := startBlockNum; blockNum <= bestBlockNum; blockNum++ {
		var nextBlockChan chan *blockResult
		if blockNum < bestBlockNum {
			nextBlockChan = make(chan *blockResult, 1)
			go func(num uint32) {
				blk, err := snapshotBestChain.GetBlock(num)
				nextBlockChan <- &blockResult{block: blk, err: err}
			}(blockNum + 1)
		}

		var snapshotBlock *block.Block
		if prefetchedBlock != nil {
			for prefetchedBlock.block == nil && prefetchedBlock.err == nil {
			}
			if prefetchedBlock.err != nil {
				return errors.Wrapf(prefetchedBlock.err, "get block %d from snapshot", blockNum)
			}
			snapshotBlock = prefetchedBlock.block
		} else {
			var err error
			snapshotBlock, err = snapshotBestChain.GetBlock(blockNum)
			if err != nil {
				return errors.Wrapf(err, "get block %d from snapshot", blockNum)
			}
		}

		parentID := snapshotBlock.Header().ParentID()
		parentSummaryChan := make(chan *summaryResult, 1)
		go func(pid thor.Bytes32) {
			summary, err := targetRepo.GetBlockSummary(pid)
			parentSummaryChan <- &summaryResult{summary: summary, err: err}
		}(parentID)

		parentResult := <-parentSummaryChan
		if parentResult.err != nil {
			return errors.Wrapf(parentResult.err, "get parent summary for block %d", blockNum)
		}
		parentSummary := parentResult.summary

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
			blocksProcessed := blockNum - startBlockNum + 1
			shouldCommitLogs := blocksProcessed%logBatchSize == 0 || blockNum == bestBlockNum

			if err := asyncWriter.Write(snapshotBlock, receipts, blockNum, shouldCommitLogs); err != nil {
				return errors.Wrapf(err, "write logs for block %d", blockNum)
			}

			select {
			case err := <-asyncWriter.errChan:
				return errors.Wrapf(err, "async log write error at block %d", blockNum)
			default:
			}
		}

		if blockNum%10000 == 0 || blockNum%100000 == 0 || blockNum == bestBlockNum {
			percent := float64(blockNum) / float64(bestBlockNum) * 100
			remaining := bestBlockNum - blockNum
			log.Info("Reprocessing progress",
				"block", blockNum,
				"total", bestBlockNum,
				"percent", fmt.Sprintf("%.2f%%", percent),
				"remaining", remaining)
			if blockNum%100000 == 0 || blockNum == bestBlockNum {
				log.Info("[REPROCESS]",
					"Block", blockNum,
					"bestBlockNum", bestBlockNum,
					"percent", percent,
					"remaining", remaining)
			}
		}

		if nextBlockChan != nil {
			prefetchedBlock = <-nextBlockChan
		} else {
			prefetchedBlock = nil
		}
	}

	log.Info("Reprocessing completed successfully", "totalBlocks", bestBlockNum+1, "remaining", totalBlocksToProcess)
	return nil
}

// detectGenesisFromSnapshot detects genesis block, fork config, and genesis builder from snapshot
func detectGenesisFromSnapshot(db *muxdb.MuxDB) (*block.Block, *thor.ForkConfig, *genesis.Genesis, error) {
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

func getInputDB(inputDataDir string) (*muxdb.MuxDB, error) {
	log.Info("Opening input database", "dir", inputDataDir)
	inputDBPath := filepath.Join(inputDataDir, "main.db")
	if _, err := os.Stat(inputDBPath); err != nil {
		return nil, errors.Wrapf(err, "input database not found at %s", inputDBPath)
	}

	opts := muxdb.Options{
		TrieNodeCacheSizeMB:        2048,
		TrieCachedNodeTTL:          60,
		TrieDedupedPartitionFactor: math.MaxUint32,
		TrieWillCleanHistory:       false,
		OpenFilesCacheCapacity:     20000,
		ReadCacheMB:                2048,
		WriteBufferMB:              4,
		TrieHistPartitionFactor:    524288,
	}

	inputDB, err := muxdb.Open(inputDBPath, &opts)
	if err != nil {
		return nil, errors.Wrap(err, "open snapshot database")
	}
	return inputDB, nil
}

func getOutputDB(outputDatDir string) (*muxdb.MuxDB, error) {
	fdCache := 5120
	cacheMB := 256

	opts := muxdb.Options{
		TrieNodeCacheSizeMB:        cacheMB,
		TrieCachedNodeTTL:          30,
		TrieDedupedPartitionFactor: math.MaxUint32,
		TrieWillCleanHistory:       true,
		OpenFilesCacheCapacity:     fdCache,
		ReadCacheMB:                128,
		WriteBufferMB:              512,
	}

	// go-ethereum stuff
	// Ensure Go's GC ignores the database cache for trigger percentage
	totalCacheMB := cacheMB + opts.ReadCacheMB + opts.WriteBufferMB*2
	gogc := math.Max(10, math.Min(100, 50/(float64(totalCacheMB)/1024)))

	log.Debug("sanitize Go's GC trigger", "percent", int(gogc))
	debug.SetGCPercent(int(gogc))

	if opts.TrieWillCleanHistory {
		opts.TrieHistPartitionFactor = 256
	} else {
		opts.TrieHistPartitionFactor = 524288
	}

	path := filepath.Join(outputDatDir, "main.db")
	db, err := muxdb.Open(path, &opts)
	if err != nil {
		return nil, errors.Wrapf(err, "open main database [%v]", path)
	}
	return db, nil
}

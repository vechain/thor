// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package reprocess

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/cmd/thor/bandwidth"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/consensus"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/logdb"
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
	wg        sync.WaitGroup
	closed    atomic.Bool
	firstErr  atomic.Pointer[error]
}

type logWriteRequest struct {
	block    *block.Block
	receipts tx.Receipts
	commit   bool
}

func newAsyncLogWriter(writer *logdb.Writer) *asyncLogWriter {
	aw := &asyncLogWriter{
		writer:    writer,
		writeChan: make(chan logWriteRequest, 100), // buffer for smooth operation
	}

	aw.wg.Add(1)
	go aw.writeLoop()
	return aw
}

func (aw *asyncLogWriter) writeLoop() {
	defer aw.wg.Done()

	// Process all pending writes until channel is closed or error occurs
	for req := range aw.writeChan {
		// Check if there's already an error, stop processing if so
		if err := aw.firstErr.Load(); err != nil {
			return
		}

		// Write logs
		if err := aw.writer.Write(req.block, req.receipts); err != nil {
			aw.firstErr.CompareAndSwap(nil, &err)
			return // Stop processing on error
		}

		// Commit if requested
		if req.commit {
			if err := aw.writer.Commit(); err != nil {
				aw.firstErr.CompareAndSwap(nil, &err)
				return // Stop processing on error
			}
		}
	}
}

func (aw *asyncLogWriter) Write(block *block.Block, receipts tx.Receipts, commit bool) error {
	// Check if writer is closed
	if aw.closed.Load() {
		return errors.New("log writer is closed")
	}

	// Check for previous errors
	if err := aw.firstErr.Load(); err != nil {
		return *err
	}

	// Send write request to channel
	// This blocks if channel is full, ensuring back pressure
	aw.writeChan <- logWriteRequest{block: block, receipts: receipts, commit: commit}
	return nil
}

func (aw *asyncLogWriter) Close() error {
	// Mark as closed to prevent new writes
	aw.closed.Store(true)

	// Close channel and wait for all pending writes to complete
	close(aw.writeChan)
	aw.wg.Wait()

	// Check if there was an error during writing
	if err := aw.firstErr.Load(); err != nil {
		return *err
	}

	// Final commit to ensure all data is persisted
	return aw.writer.Commit()
}

// ReprocessChainFromSnapshot reprocesses all blocks from a snapshot data directory
func ReprocessChainFromSnapshot(
	ctx context.Context,
	source *chain.Chain,
	stater *state.Stater,
	logDB *logdb.LogDB,
	repo *chain.Repository,
	forkConfig *thor.ForkConfig,
	bftEngine bft.Committer,
	skipLogs bool,
) error {
	// Get start and end block numbers
	bestBlockSummary := repo.BestBlockSummary()
	startBlockNum := bestBlockSummary.Header.Number() + 1
	endBlockNumber := block.Number(source.HeadID())

	if startBlockNum > endBlockNumber {
		log.Info("reprocessing already complete", "currentBlock", startBlockNum-1, "sourceBlock", endBlockNumber)
		return nil
	}

	totalBlocks := endBlockNumber - startBlockNum + 1
	log.Info("starting reprocessing", "startBlock", startBlockNum, "endBlock", endBlockNumber, "totalBlocks", totalBlocks)

	// Create consensus
	cons := consensus.New(repo, stater, forkConfig)

	// Setup async log writer
	var asyncWriter *asyncLogWriter
	if !skipLogs {
		logWriter := logDB.NewWriter()
		asyncWriter = newAsyncLogWriter(logWriter)
		defer func() {
			if asyncWriter != nil {
				if err := asyncWriter.Close(); err != nil {
					log.Error("failed to close async log writer", "err", err)
				}
			}
		}()
	}

	// Three-stage pipeline for block reprocessing:
	//
	// Stage 1: Block Fetcher
	//   - Fetches blocks from source chain
	//   - Sends to rawBlockChan
	//
	// Stage 2: Block Warmer
	//   - Receives blocks from rawBlockChan
	//   - Pre-warms caches (ID, Beta, IntrinsicGas, etc.)
	//   - Sends warmed blocks to blockChan
	//
	// Stage 3: Block Processor (main loop)
	//   - Receives warmed blocks from blockChan
	//   - Processes blocks (consensus, state, logs)
	//
	rawBlockChan := make(chan *block.Block, 100)
	blockChan := make(chan *block.Block, 1000)
	errChan := make(chan error, 1)

	// Stage 1: Fetch blocks from source
	go func() {
		defer close(rawBlockChan)
		for blockNum := startBlockNum; blockNum <= endBlockNumber; blockNum++ {
			// Check for cancellation before expensive GetBlock call
			select {
			case <-ctx.Done():
				return // Stop producing, channel will be closed by defer
			default:
			}

			blk, err := source.GetBlock(blockNum)
			if err != nil {
				errChan <- errors.Wrapf(err, "fetch block %d from source", blockNum)
				return
			}

			select {
			case rawBlockChan <- blk:
			case <-ctx.Done():
				return // Stop producing
			}
		}
	}()

	// Stage 2: Warmup blocks
	go func() {
		defer close(blockChan)
		<-co.Parallel(func(queue chan<- func()) {
			for {
				// Non-blocking receive with cancellation support
				var blk *block.Block
				select {
				case b, ok := <-rawBlockChan:
					if !ok {
						return // Channel closed, done
					}
					blk = b
				case <-ctx.Done():
					return // Stop producing
				}

				// Warmup: precompute and cache expensive operations
				queue <- func() {
					_ = blk.Header().ID()
					_, _ = blk.Header().Beta()
				}
				txs := blk.Transactions()
				queue <- func() {
					for _, tx := range txs {
						_ = tx.ID()
						_ = tx.UnprovedWork()
						_, _ = tx.IntrinsicGas()
						_, _ = tx.Delegator()
					}
				}

				select {
				case blockChan <- blk:
				case <-ctx.Done():
					return // Stop producing
				}
			}
		})
	}()

	// Stage 3: Process blocks
	blocksProcessed := uint32(0)
	bandwidth := &bandwidth.Bandwidth{}
	processStartTime := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errChan:
			return err
		case blk, ok := <-blockChan:
			if !ok {
				// Channel closed, all blocks processed
				log.Info(
					"reprocessing completed successfully",
					"processed",
					blocksProcessed,
					"best",
					repo.BestBlockSummary().Header.Number(),
					"duration",
					time.Since(processStartTime),
				)
				return nil
			}

			blockNum := blk.Header().Number()

			// Get parent summary
			parentID := blk.Header().ParentID()
			parentSummary, err := repo.GetBlockSummary(parentID)
			if err != nil {
				return errors.Wrapf(err, "get parent summary for block %d", blockNum)
			}
			startTime := time.Now()
			// Process block
			stage, receipts, err := cons.Process(parentSummary, blk, blk.Header().Timestamp(), 0)
			if err != nil {
				return errors.Wrapf(err, "process block %d", blockNum)
			}

			// Commit state
			if _, err := stage.Commit(); err != nil {
				return errors.Wrapf(err, "commit state for block %d", blockNum)
			}

			// Add block to repository
			if err := repo.AddBlock(blk, receipts, 0, true); err != nil {
				return errors.Wrapf(err, "add block %d to repository", blockNum)
			}

			// commit block in bft engine
			if blk.Header().Number() >= forkConfig.FINALITY {
				if err := bftEngine.CommitBlock(blk.Header(), false); err != nil {
					return errors.Wrap(err, "bft commits")
				}
			}

			realElapsed := time.Since(startTime)
			bandwidth.Update(blk.Header(), realElapsed)

			// Write logs
			if !skipLogs {
				blocksProcessed++
				shouldCommit := blocksProcessed%logBatchSize == 0 || blockNum == endBlockNumber
				if err := asyncWriter.Write(blk, receipts, shouldCommit); err != nil {
					return errors.Wrapf(err, "write logs for block %d", blockNum)
				}
			}

			// Progress logging
			if blockNum%10000 == 0 || blockNum == endBlockNumber {
				percent := float64(blockNum-startBlockNum+1) / float64(totalBlocks) * 100
				log.Info("processed blocks",
					"block", blockNum,
					"bandwidth", fmt.Sprintf("%.2f mgas/s", float64(bandwidth.Value())/1000000),
					"percent", fmt.Sprintf("%.2f%%", percent))
			}
		}
	}
}

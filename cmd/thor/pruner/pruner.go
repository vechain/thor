// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pruner

import (
	"context"
	"fmt"
	"time"

	"github.com/vechain/thor/v2/block"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"

	"github.com/vechain/thor/v2/bft"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/co"
	"github.com/vechain/thor/v2/log"
	"github.com/vechain/thor/v2/muxdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/trie"
)

var logger = log.WithContext("pkg", "pruner")

const (
	propsStoreName = "pruner.props"
	statusKey      = "status"
)

// Pruner is a background task to prune tries.
type Pruner struct {
	db       *muxdb.MuxDB
	repo     *chain.Repository
	ctx      context.Context
	commiter bft.Committer
	cancel   func()
	goes     co.Goes
}

// New creates and starts the pruner.
func New(db *muxdb.MuxDB, repo *chain.Repository, commiter bft.Committer) *Pruner {
	ctx, cancel := context.WithCancel(context.Background())
	o := &Pruner{
		db:       db,
		repo:     repo,
		ctx:      ctx,
		commiter: commiter,
		cancel:   cancel,
	}
	o.goes.Go(func() {
		if err := o.loop(); err != nil {
			if err != context.Canceled && errors.Cause(err) != context.Canceled {
				logger.Warn("pruner interrupted", "error", err)
			}
		}
	})
	return o
}

// Stop stops the pruner.
func (p *Pruner) Stop() {
	p.cancel()
	p.goes.Wait()
}

// loop is the main loop.
func (p *Pruner) loop() error {
	logger.Info("pruner started")

	var (
		status     status
		propsStore = p.db.NewStore(propsStoreName)
	)
	if err := status.Load(propsStore); err != nil {
		return errors.Wrap(err, "load status")
	}

	for {
		period := uint32(65536)
		if int64(p.repo.BestBlockSummary().Header.Timestamp()) > time.Now().Unix()-10*24*3600 {
			// use smaller period when nearly synced
			period = 8192
		}

		// select target
		target := status.Base + period

		targetChain, err := p.awaitUntilSteady(target + thor.MaxStateHistory)
		if err != nil {
			return errors.Wrap(err, "awaitUntilSteady")
		}
		startTime := time.Now().UnixNano()

		// prune index/account/storage tries
		if err := p.pruneTries(targetChain, status.Base, target); err != nil {
			return errors.Wrap(err, "prune tries")
		}

		logger.Info("prune tries",
			"range", fmt.Sprintf("#%v+%v", status.Base, target-status.Base),
			"et", time.Duration(time.Now().UnixNano()-startTime),
		)

		status.Base = target
		if err := status.Save(propsStore); err != nil {
			return errors.Wrap(err, "save status")
		}
	}
}

// newStorageTrieIfUpdated creates a storage trie object from the account leaf if the storage trie updated since base.
func (p *Pruner) newStorageTrieIfUpdated(accLeaf *trie.Leaf, base uint32) *muxdb.Trie {
	if len(accLeaf.Meta) == 0 {
		return nil
	}

	var (
		acc  state.Account
		meta state.AccountMetadata
	)
	if err := rlp.DecodeBytes(accLeaf.Value, &acc); err != nil {
		panic(errors.Wrap(err, "decode account"))
	}

	if err := rlp.DecodeBytes(accLeaf.Meta, &meta); err != nil {
		panic(errors.Wrap(err, "decode account metadata"))
	}

	if meta.StorageMajorVer >= base {
		return p.db.NewTrie(
			state.StorageTrieName(meta.StorageID),
			trie.Root{
				Hash: thor.BytesToBytes32(acc.StorageRoot),
				Ver: trie.Version{
					Major: meta.StorageMajorVer,
					Minor: meta.StorageMinorVer,
				},
			})
	}
	return nil
}

// checkpointTries transfers tries' standalone nodes, whose major version within [base, target).
func (p *Pruner) checkpointTries(targetChain *chain.Chain, base, target uint32) error {
	summary, err := targetChain.GetBlockSummary(target - 1)
	if err != nil {
		return err
	}

	// checkpoint index trie
	indexTrie := p.db.NewTrie(chain.IndexTrieName, summary.IndexRoot())
	indexTrie.SetNoFillCache(true)

	if err := indexTrie.Checkpoint(p.ctx, base, nil); err != nil {
		return err
	}

	// checkpoint account trie
	accTrie := p.db.NewTrie(state.AccountTrieName, summary.Root())
	accTrie.SetNoFillCache(true)

	var sTries []*muxdb.Trie
	if err := accTrie.Checkpoint(p.ctx, base, func(leaf *trie.Leaf) {
		if sTrie := p.newStorageTrieIfUpdated(leaf, base); sTrie != nil {
			sTries = append(sTries, sTrie)
		}
	}); err != nil {
		return err
	}

	// checkpoint storage tries
	for _, sTrie := range sTries {
		sTrie.SetNoFillCache(true)
		if err := sTrie.Checkpoint(p.ctx, base, nil); err != nil {
			return err
		}
	}
	return nil
}

// pruneTries prunes index/account/storage tries in the range [base, target).
func (p *Pruner) pruneTries(targetChain *chain.Chain, base, target uint32) error {
	if err := p.checkpointTries(targetChain, base, target); err != nil {
		return errors.Wrap(err, "checkpoint tries")
	}

	if err := p.db.DeleteTrieHistoryNodes(p.ctx, base, target); err != nil {
		return errors.Wrap(err, "delete trie history")
	}
	return nil
}

// awaitUntilSteady waits until the target block number becomes almost final(steady),
// and returns the steady chain.
func (p *Pruner) awaitUntilSteady(target uint32) (*chain.Chain, error) {
	for {
		finalizedID := p.commiter.Finalized()
		finalizedNum := block.Number(finalizedID)

		// If target is behind or equal to finalized checkpoint, it's safe to prune
		if target <= finalizedNum {
			// Get the block ID for the target (or use finalized if target is beyond it)
			best := p.repo.BestBlockSummary()
			bestChain := p.repo.NewChain(best.Header.ID())

			// Ensure the target block exists on the best chain
			targetID, err := bestChain.GetBlockID(target)
			if err != nil {
				// If target doesn't exist yet, wait
				select {
				case <-p.ctx.Done():
					return nil, p.ctx.Err()
				case <-time.After(time.Second):
					continue
				}
			}

			// Verify the target block is on the same chain as finalized
			// (i.e., target is an ancestor of finalized, or they're the same block)
			finalizedChain := p.repo.NewChain(finalizedID)

			if finalizedNum > 0 {
				hasTarget, err := finalizedChain.HasBlock(targetID)
				if err != nil {
					return nil, err
				}
				if !hasTarget {
					// Target is on a different branch, wait for chain to reorganize
					select {
					case <-p.ctx.Done():
						return nil, p.ctx.Err()
					case <-time.After(time.Second):
						continue
					}
				}
			}

			targetChain := p.repo.NewChain(targetID)
			return targetChain, nil
		}

		// Wait for finality to advance
		select {
		case <-p.ctx.Done():
			return nil, p.ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

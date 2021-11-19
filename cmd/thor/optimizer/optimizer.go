// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package optimizer

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
	"github.com/vechain/thor/trie"
)

var log = log15.New("pkg", "optimizer")

const (
	propsStoreName = "optimizer.props"
	statusKey      = "status"

	minPeriod = 1800  // 5 hours
	maxPeriod = 18000 // 50 hours
)

// Optimizer is a background task to optimize tries.
type Optimizer struct {
	db     *muxdb.MuxDB
	repo   *chain.Repository
	ctx    context.Context
	cancel func()
	goes   co.Goes
}

// New creates and starts the optimizer.
func New(db *muxdb.MuxDB, repo *chain.Repository, prune bool) *Optimizer {
	ctx, cancel := context.WithCancel(context.Background())
	o := &Optimizer{
		db:     db,
		repo:   repo,
		ctx:    ctx,
		cancel: cancel,
	}
	o.goes.Go(func() {
		if err := o.loop(prune); err != nil {
			if err != context.Canceled && errors.Cause(err) != context.Canceled {
				log.Warn("optimizer interrupted", "error", err)
			}
		}
	})
	return o
}

// Stop stops the optimizer.
func (p *Optimizer) Stop() {
	p.cancel()
	p.goes.Wait()
}

// loop is the main loop.
func (p *Optimizer) loop(prune bool) error {
	log.Info("optimizer started")

	var (
		status      status
		lastLogTime = time.Now().UnixNano()
		propsStore  = p.db.NewStore(propsStoreName)
	)
	if err := status.Load(propsStore); err != nil {
		return errors.Wrap(err, "load stats")
	}

	for {
		// select target
		target := p.repo.BestBlockSummary().Header.Number()
		if min, max := status.Base+minPeriod, status.Base+maxPeriod; target < min {
			target = min
		} else if target > max {
			target = max
		}

		targetChain, err := p.awaitUntilSteady(target)
		if err != nil {
			return errors.Wrap(err, "waitUntilSteady")
		}
		startTime := time.Now().UnixNano()
		if err := p.alignToPartition(p.db.TrieDedupedPartitionFactor(), status.Base, target, func(alignedBase, alignedTarget uint32) error {
			alignedSummary, err := targetChain.GetBlockSummary(alignedTarget)
			if err != nil {
				return errors.Wrap(err, "GetBlockSummary")
			}
			// no need to update leaf bank for the index trie
			if prune {
				// prune the index trie
				indexTrie := p.db.NewTrie(chain.IndexTrieName, alignedSummary.IndexRoot, alignedSummary.Header.Number())
				indexTrie.SetNoFillCache(true)
				if err := indexTrie.DumpAndCleanNodes(p.ctx, alignedBase); err != nil {
					return errors.Wrap(err, "prune index trie")
				}
			}
			accTrie := p.db.NewTrie(state.AccountTrieName, alignedSummary.Header.StateRoot(), alignedSummary.Header.Number())
			accTrie.SetNoFillCache(true)

			var updatedStorageTries []*muxdb.Trie
			if err := accTrie.DumpLeaves(p.ctx, alignedBase, func(leaf *trie.Leaf) *trie.Leaf {
				var acc state.Account
				if err := rlp.DecodeBytes(leaf.Value, &acc); err != nil {
					panic(errors.Wrap(err, "decode account"))
				}
				am := state.AccountMetadata(leaf.Meta)
				storageCommitNum := am.StorageCommitNum()
				addr, ok := am.Address()
				if !ok {
					panic(errors.New("account metadata: missing address"))
				}
				// collect updated storage tries
				if storageCommitNum >= alignedBase && len(acc.StorageRoot) > 0 {
					updatedStorageTries = append(
						updatedStorageTries,
						p.db.NewTrie(
							state.StorageTrieName(addr, am.StorageInitCommitNum()),
							thor.BytesToBytes32(acc.StorageRoot),
							storageCommitNum,
						))
				}
				return &trie.Leaf{
					Value: leaf.Value,
					Meta:  am.SkipAddress(), // skip address to save space
				}
			}); err != nil {
				return errors.Wrap(err, "dump account trie leaves")
			}

			for _, sTrie := range updatedStorageTries {
				sTrie.SetNoFillCache(true)
				if err := sTrie.DumpLeaves(p.ctx, alignedBase, transformStorageLeaf); err != nil {
					return errors.Wrap(err, "dump storage trie leaves")
				}
				if prune {
					if err := sTrie.DumpAndCleanNodes(p.ctx, alignedBase); err != nil {
						return errors.Wrap(err, "prune storage trie")
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}

		// prune the account trie.
		if prune && target > thor.MaxStateHistory {
			accountTarget := target - thor.MaxStateHistory
			if err := p.alignToPartition(p.db.TrieDedupedPartitionFactor(), status.AccountBase, accountTarget, func(alignedBase, alignedTarget uint32) error {
				alignedHeader, err := targetChain.GetBlockHeader(alignedTarget)
				if err != nil {
					return errors.Wrap(err, "GetBlockHeader")
				}
				accTrie := p.db.NewTrie(state.AccountTrieName, alignedHeader.StateRoot(), alignedHeader.Number())
				accTrie.SetNoFillCache(true)

				if err := accTrie.DumpAndCleanNodes(p.ctx, alignedBase); err != nil {
					return errors.Wrap(err, "prune account trie")
				}
				return nil
			}); err != nil {
				return err
			}
			status.AccountBase = accountTarget
		}
		if now := time.Now().UnixNano(); now-lastLogTime > int64(time.Second*20) {
			lastLogTime = now
			log.Info("optimized tries",
				"range", fmt.Sprintf("#%v+%v", status.Base, target-status.Base),
				"et", time.Duration(now-startTime),
			)
		}
		status.Base = target
		if err := status.Save(propsStore); err != nil {
			return errors.Wrap(err, "save status")
		}
	}
}

// alignToPartition aligns to deduped node partition. Aligned partition becomes a checkpoint.
func (p *Optimizer) alignToPartition(triePtnFactor muxdb.TriePartitionFactor, base, target uint32, f func(alignedBase, alignedTarget uint32) error) error {
	for {
		pid := triePtnFactor.Which(base)
		_, limit := triePtnFactor.Range(pid)

		if limit > target {
			limit = target
		}

		if err := f(base, limit); err != nil {
			return err
		}

		if target == limit {
			return nil
		}
		base = limit + 1
	}
}

// awaitUntilSteady waits until the target block number becomes almost final(steady),
// and returns the steady chain.
func (p *Optimizer) awaitUntilSteady(target uint32) (*chain.Chain, error) {
	// the knowned steady id is newer than target
	if steadyID := p.repo.SteadyBlockID(); block.Number(steadyID) >= target {
		return p.repo.NewChain(steadyID), nil
	}

	const windowSize = 8640 * 14 // about 2 weeks

	backoff := uint32(thor.MaxBlockProposers)
	for {
		best := p.repo.BestBlockSummary()
		bestNum := best.Header.Number()
		if bestNum > target+backoff && bestNum > windowSize {
			refNum := bestNum - windowSize
			refHeader, err := p.repo.NewChain(best.Header.ID()).GetBlockHeader(refNum)
			if err != nil {
				return nil, err
			}
			meanScore := int(math.Round(float64(best.Header.TotalScore()-refHeader.TotalScore()) / float64(windowSize)))
			set := make(map[thor.Address]struct{})
			// reverse iterate the chain and collect signers.
			for i, prev := 0, best.Header; i < meanScore*3 && prev.Number() >= target; i++ {
				signer, _ := prev.Signer()
				set[signer] = struct{}{}
				if len(set) >= (meanScore+1)/2 {
					// got enough unique signers
					steadyID := prev.ID()
					if err := p.repo.SetSteadyBlockID(steadyID); err != nil {
						return nil, err
					}
					return p.repo.NewChain(steadyID), nil
				}
				parent, err := p.repo.GetBlockSummary(prev.ParentID())
				if err != nil {
					return nil, err
				}
				prev = parent.Header
			}
			backoff += uint32(meanScore)
		} else {
			select {
			case <-p.ctx.Done():
				return nil, p.ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
}

// transformStorageLeaf transforms storage leaf before saving into leaf bank.
// Leaf bank is for point read, so the metadata which is the preimage of storage key,
// can be fully dropped to save disk space.
func transformStorageLeaf(leaf *trie.Leaf) *trie.Leaf {
	return &trie.Leaf{Value: leaf.Value}
}

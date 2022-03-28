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

	const (
		period        = 2000  // the period to update leafbank.
		prunePeriod   = 10000 // the period to prune tries.
		pruneReserved = 70000 // must be > thor.MaxStateHistory
	)

	var (
		status      status
		lastLogTime = time.Now().UnixNano()
		propsStore  = p.db.NewStore(propsStoreName)
	)
	if err := status.Load(propsStore); err != nil {
		return errors.Wrap(err, "load status")
	}

	for {
		// select target
		target := status.Base + period

		targetChain, err := p.awaitUntilSteady(target)
		if err != nil {
			return errors.Wrap(err, "awaitUntilSteady")
		}
		startTime := time.Now().UnixNano()

		// dump account/storage trie leaves into leafbank
		if err := p.dumpStateLeaves(targetChain, status.Base, target); err != nil {
			return errors.Wrap(err, "dump state trie leaves")
		}

		// prune index/account/storage tries
		if prune && target > pruneReserved {
			if pruneTarget := target - pruneReserved; pruneTarget >= status.PruneBase+prunePeriod {
				if err := p.pruneTries(targetChain, status.PruneBase, pruneTarget); err != nil {
					return errors.Wrap(err, "prune tries")
				}
				status.PruneBase = pruneTarget
			}
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

// newStorageTrieIfUpdated creates a storage trie object from the account leaf if the storage trie updated since base.
func (p *Optimizer) newStorageTrieIfUpdated(accLeaf *trie.Leaf, base uint32) *muxdb.Trie {
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

	if meta.StorageCommitNum >= base {
		return p.db.NewTrie(
			state.StorageTrieName(meta.StorageID),
			thor.BytesToBytes32(acc.StorageRoot),
			meta.StorageCommitNum,
			meta.StorageDistinctNum,
		)
	}
	return nil
}

// dumpStateLeaves dumps account/storage trie leaves updated within [base, target) into leafbank.
func (p *Optimizer) dumpStateLeaves(targetChain *chain.Chain, base, target uint32) error {
	h, err := targetChain.GetBlockSummary(target - 1)
	if err != nil {
		return err
	}
	accTrie := p.db.NewTrie(state.AccountTrieName, h.Header.StateRoot(), h.Header.Number(), h.Conflicts)
	accTrie.SetNoFillCache(true)

	var sTries []*muxdb.Trie
	if err := accTrie.DumpLeaves(p.ctx, base, h.Header.Number(), func(leaf *trie.Leaf) *trie.Leaf {
		if sTrie := p.newStorageTrieIfUpdated(leaf, base); sTrie != nil {
			sTries = append(sTries, sTrie)
		}
		return leaf
	}); err != nil {
		return err
	}
	for _, sTrie := range sTries {
		sTrie.SetNoFillCache(true)
		if err := sTrie.DumpLeaves(p.ctx, base, h.Header.Number(), func(leaf *trie.Leaf) *trie.Leaf {
			return &trie.Leaf{Value: leaf.Value} // skip metadata to save space
		}); err != nil {
			return err
		}
	}
	return nil
}

// dumpTrieNodes dumps index/account/storage trie nodes committed within [base, target] into deduped space.
func (p *Optimizer) dumpTrieNodes(targetChain *chain.Chain, base, target uint32) error {
	summary, err := targetChain.GetBlockSummary(target - 1)
	if err != nil {
		return err
	}

	// dump index trie
	indexTrie := p.db.NewNonCryptoTrie(chain.IndexTrieName, trie.NonCryptoNodeHash, summary.Header.Number(), summary.Conflicts)
	indexTrie.SetNoFillCache(true)

	if err := indexTrie.DumpNodes(p.ctx, base, nil); err != nil {
		return err
	}

	// dump account trie
	accTrie := p.db.NewTrie(state.AccountTrieName, summary.Header.StateRoot(), summary.Header.Number(), summary.Conflicts)
	accTrie.SetNoFillCache(true)

	var sTries []*muxdb.Trie
	if err := accTrie.DumpNodes(p.ctx, base, func(leaf *trie.Leaf) {
		if sTrie := p.newStorageTrieIfUpdated(leaf, base); sTrie != nil {
			sTries = append(sTries, sTrie)
		}
	}); err != nil {
		return err
	}

	// dump storage tries
	for _, sTrie := range sTries {
		sTrie.SetNoFillCache(true)
		if err := sTrie.DumpNodes(p.ctx, base, nil); err != nil {
			return err
		}
	}
	return nil
}

// pruneTries prunes index/account/storage tries in the range [base, target).
func (p *Optimizer) pruneTries(targetChain *chain.Chain, base, target uint32) error {
	if err := p.dumpTrieNodes(targetChain, base, target); err != nil {
		return errors.Wrap(err, "dump trie nodes")
	}

	cleanBase := base
	if base == 0 {
		// keeps genesis state history like the previous version.
		cleanBase = 1
	}
	if err := p.db.CleanTrieHistory(p.ctx, cleanBase, target); err != nil {
		return errors.Wrap(err, "clean trie history")
	}
	return nil
}

// awaitUntilSteady waits until the target block number becomes almost final(steady),
// and returns the steady chain.
func (p *Optimizer) awaitUntilSteady(target uint32) (*chain.Chain, error) {
	// the knowned steady id is newer than target
	if steadyID := p.repo.SteadyBlockID(); block.Number(steadyID) >= target {
		return p.repo.NewChain(steadyID), nil
	}

	const windowSize = 100000

	backoff := uint32(0)
	for {
		best := p.repo.BestBlockSummary()
		bestNum := best.Header.Number()
		if bestNum > target+backoff {
			var meanScore float64
			if bestNum > windowSize {
				baseNum := bestNum - windowSize
				baseHeader, err := p.repo.NewChain(best.Header.ID()).GetBlockHeader(baseNum)
				if err != nil {
					return nil, err
				}
				meanScore = math.Round(float64(best.Header.TotalScore()-baseHeader.TotalScore()) / float64(windowSize))
			} else {
				meanScore = math.Round(float64(best.Header.TotalScore()) / float64(bestNum))
			}
			set := make(map[thor.Address]struct{})
			// reverse iterate the chain and collect signers.
			for i, prev := 0, best.Header; i < int(meanScore*3) && prev.Number() >= target; i++ {
				signer, _ := prev.Signer()
				set[signer] = struct{}{}
				if len(set) >= int(math.Round((meanScore+1)/2)) {
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

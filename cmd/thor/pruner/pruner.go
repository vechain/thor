// Copyright (c) 2019 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package pruner

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/inconshreveable/log15"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/co"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

var log = log15.New("pkg", "pruner")

const (
	propsStoreName = "pruner.props"
	statusKey      = "status"
)

type counter struct {
	nIndexTrieNodeDeleted,
	nAccountTrieNodeDeleted,
	nStorageTrieNodeDeleted int
}

// Pruner is the state pruner.
type Pruner struct {
	db     *muxdb.MuxDB
	repo   *chain.Repository
	ctx    context.Context
	cancel func()
	goes   co.Goes
}

// New creates and starts a state pruner.
func New(db *muxdb.MuxDB, repo *chain.Repository) *Pruner {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pruner{
		db:     db,
		repo:   repo,
		ctx:    ctx,
		cancel: cancel,
	}
	p.goes.Go(func() {
		if err := p.loop(); err != nil {
			if err != context.Canceled {
				log.Warn("pruner interrupted", "error", err)
			}
		}
	})
	return p
}

// Stop stops the state pruner.
func (p *Pruner) Stop() {
	p.cancel()
	p.goes.Wait()
}

// loop the main loop.
func (p *Pruner) loop() error {
	log.Info("pruner started")
	const (
		minSpan = 720   // 2 hours
		maxSpan = 18000 // 50 hours
	)

	var (
		status  status
		target  uint32
		counter counter
	)

	if err := status.Load(p.db); err != nil {
		return err
	}

	p.goes.Go(func() { p.statsLoop(&counter, &status.Base, &target) })

	for {
		// select target
		target = p.repo.BestBlock().Header().Number()
		if target < status.Base+minSpan {
			target = status.Base + minSpan
		} else if target > status.Base+maxSpan {
			target = status.Base + maxSpan
		}

		targetChain, err := p.waitUntilAlmostFinal(target, &status)
		if err != nil {
			return err
		}

		summary, err := targetChain.GetBlockSummary(target)
		if err != nil {
			return err
		}
		// prune the index trie
		indexTrie := p.db.NewTrie(chain.IndexTrieName, summary.IndexRoot, target)
		if nDeleted, err := indexTrie.Prune(p.ctx, status.Base); err != nil {
			return err
		} else {
			counter.nIndexTrieNodeDeleted += nDeleted
		}

		// prune the account trie and storage tries
		if err := p.pruneState(targetChain, status.Base, target, summary.Header.StateRoot(), &counter); err != nil {
			return err
		}

		status.Base = target
		if err := status.Save(p.db); err != nil {
			return err
		}
	}
}

// pruneState prunes the account trie and storage tries.
func (p *Pruner) pruneState(targetChain *chain.Chain, base, target uint32, stateRoot thor.Bytes32, counter *counter) error {
	accIter := p.db.NewTrie(state.AccountTrieName, stateRoot, target).
		NodeIterator(nil, func(path []byte, commitNum uint32) bool {
			return commitNum > base
		})

	// iterate updated accounts since the base, and prune storage tries
	for accIter.Next(true) {
		if leaf := accIter.Leaf(); leaf != nil {
			var acc state.Account
			if err := rlp.DecodeBytes(leaf.Value, &acc); err != nil {
				return err
			}
			if len(acc.StorageRoot) == 0 {
				// skip, no storage
				continue
			}

			var meta state.AccountMetadata
			if err := rlp.DecodeBytes(leaf.Meta, &meta); err != nil {
				return err
			}
			if meta.StorageCommitNum <= base {
				// skip, no storage updates
				continue
			}
			sTrie := p.db.NewTrie(
				state.StorageTrieName(meta.Addr),
				thor.BytesToBytes32(acc.StorageRoot),
				meta.StorageCommitNum,
			)
			if nDeleted, err := sTrie.Prune(p.ctx, base); err != nil {
				return err
			} else {
				counter.nStorageTrieNodeDeleted += nDeleted
			}
		}
	}
	if err := accIter.Error(); err != nil {
		return err
	}

	if base > thor.MaxStateHistory {
		aBase := base - thor.MaxStateHistory
		aTarget := target - thor.MaxStateHistory
		summary, err := targetChain.GetBlockSummary(aTarget)
		if err != nil {
			return err
		}
		// prune the account trie
		accTrie := p.db.NewTrie(state.AccountTrieName, summary.Header.StateRoot(), aTarget)
		if nDeleted, err := accTrie.Prune(p.ctx, aBase); err != nil {
			return err
		} else {
			counter.nAccountTrieNodeDeleted += nDeleted
		}
	}
	return nil
}

// statsLoop the loop prints stats logs.
func (p *Pruner) statsLoop(counter *counter, base, target *uint32) {
	isNearlySynced := func() bool {
		diff := time.Now().Unix() - int64(p.repo.BestBlock().Header().Timestamp())
		if diff < 0 {
			diff = -diff
		}
		return diff < 120
	}
	for {
		period := time.Second * 20
		if isNearlySynced() {
			period = time.Minute * 5
		}
		select {
		case <-p.ctx.Done():
			return
		case <-time.After(period):
		}

		log.Info("pruning tries",
			"i", counter.nIndexTrieNodeDeleted,
			"a", counter.nAccountTrieNodeDeleted,
			"s", counter.nStorageTrieNodeDeleted,
			"r", fmt.Sprintf("#%v+%v", *base, *target-*base),
		)
	}
}

func (p *Pruner) getProposerCount(header *block.Header) (int, error) {
	st := state.New(p.db, header.StateRoot(), header.Number())
	endorsement, err := builtin.Params.Native(st).Get(thor.KeyProposerEndorsement)
	if err != nil {
		return 0, err
	}

	candidates, err := builtin.Authority.Native(st).Candidates(endorsement, thor.MaxBlockProposers)
	if err != nil {
		return 0, err
	}
	return len(candidates), nil
}

// waitUntilAlmostFinal waits until the target block number becomes almost final,
// and returns the canonical chain.
func (p *Pruner) waitUntilAlmostFinal(target uint32, status *status) (*chain.Chain, error) {
	if target <= status.Top {
		return p.repo.NewBestChain(), nil
	}

	ticker := p.repo.NewTicker()
	for {
		best := p.repo.BestBlock()
		// requires the best block number larger enough than target.
		if best.Header().Number() > target+uint32(thor.MaxBlockProposers*2) {
			proposerCount, err := p.getProposerCount(best.Header())
			if err != nil {
				return nil, err
			}

			set := make(map[thor.Address]struct{})
			h := best.Header()
			// reverse iterate the chain and collect signers.
			for i := 0; i < int(thor.MaxBlockProposers)*5 && h.Number() > target; i++ {
				signer, _ := h.Signer()
				set[signer] = struct{}{}

				if len(set) >= (proposerCount+1)/2 {
					// got enough unique signers
					status.Top = h.Number()
					return p.repo.NewChain(best.Header().ID()), nil
				}
				s, err := p.repo.GetBlockSummary(h.ParentID())
				if err != nil {
					return nil, err
				}
				h = s.Header
			}
		}

		select {
		case <-p.ctx.Done():
			return nil, p.ctx.Err()
		case <-time.After(time.Second):
			select {
			case <-p.ctx.Done():
				return nil, p.ctx.Err()
			case <-ticker.C():
			}
		}
	}
}

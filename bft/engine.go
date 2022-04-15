// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>
package bft

import (
	"sort"
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"github.com/vechain/thor/block"
	"github.com/vechain/thor/builtin"
	"github.com/vechain/thor/cache"
	"github.com/vechain/thor/chain"
	"github.com/vechain/thor/kv"
	"github.com/vechain/thor/muxdb"
	"github.com/vechain/thor/state"
	"github.com/vechain/thor/thor"
)

const storeName = "bft.engine"

var (
	votedKey     = []byte("bft-voted")
	committedKey = []byte("bft-committed")
)

type GetBlockHeader func(id thor.Bytes32) (*block.Header, error)
type Finalize func() error

// BFTEngine tracks all votes of blocks, computes the finalized checkpoint.
type BFTEngine struct {
	repo       *chain.Repository
	store      kv.Store
	stater     *state.Stater
	forkConfig thor.ForkConfig
	voted      map[thor.Bytes32]uint32
	committed  atomic.Value
	caches     struct {
		state   *lru.Cache
		weight  *lru.Cache
		mbp     *lru.Cache
		voteset *cache.PrioCache
	}
}

// NewEngine creates a new bft engine.
func NewEngine(repo *chain.Repository, mainDB *muxdb.MuxDB, forkConfig thor.ForkConfig) (*BFTEngine, error) {
	engine := BFTEngine{
		repo:       repo,
		store:      mainDB.NewStore(storeName),
		stater:     state.NewStater(mainDB),
		forkConfig: forkConfig,
	}

	engine.caches.state, _ = lru.New(1024)
	engine.caches.weight, _ = lru.New(1024)
	engine.caches.mbp, _ = lru.New(8)
	engine.caches.voteset = cache.NewPrioCache(16)

	voted, err := loadVoted(engine.store)
	if err != nil {
		return nil, err
	}
	engine.voted = voted

	if val, err := engine.store.Get(committedKey); err != nil {
		if !engine.store.IsNotFound(err) {
			return nil, err
		}
		engine.committed.Store(engine.repo.GenesisBlock().Header().ID())
	} else {
		engine.committed.Store(thor.BytesToBytes32(val))
	}

	return &engine, nil
}

// Committed returns the committed checkpoint, which is finalized.
func (engine *BFTEngine) Committed() thor.Bytes32 {
	return engine.committed.Load().(thor.Bytes32)
}

// Accepts checks if the given block is on the same branch of committed checkpoint.
func (engine *BFTEngine) Accepts(parentID thor.Bytes32) error {
	if block.Number(parentID) < engine.forkConfig.FINALITY {
		return nil
	}

	committed := engine.Committed()
	if block.Number(committed) == 0 {
		return nil
	}

	if included, err := engine.repo.NewChain(parentID).HasBlock(committed); err != nil {
		return err
	} else if !included {
		return errConflictWithCommitted
	}

	return nil
}

// Process processes block in bft engine and returns whether the block becomes new best.
// Not thread-safe!
func (engine *BFTEngine) Process(header *block.Header) (becomeNewBest bool, finalize Finalize, err error) {
	finalize = func() error { return nil }

	best := engine.repo.BestBlockSummary().Header
	if header.Number() < engine.forkConfig.FINALITY || best.Number() < engine.forkConfig.FINALITY {
		becomeNewBest = header.BetterThan(best)
		return
	}

	st, err := engine.getState(header.ID(), func(id thor.Bytes32) (*block.Header, error) {
		// header was not added to repo at this moment
		if id == header.ID() {
			return header, nil
		}
		return engine.repo.GetBlockHeader(id)
	})
	if err != nil {
		return
	}

	bSt, err := engine.getState(best.ID(), engine.repo.GetBlockHeader)
	if err != nil {
		return
	}

	if st.Weight != bSt.Weight {
		becomeNewBest = st.Weight > bSt.Weight
	} else {
		becomeNewBest = header.BetterThan(best)
	}

	finalize = func() error {
		// save weight at the end of round
		if (header.Number()+1)%thor.CheckpointInterval == 0 {
			if err := saveWeight(engine.store, header.ID(), st.Weight); err != nil {
				return err
			}
			engine.caches.weight.Add(header.ID(), st.Weight)
		}

		// update commmitted if new block commits this round
		if st.CommitAt != nil && header.ID() == *st.CommitAt && st.Weight > 1 {
			id, err := engine.findCheckpointByWeight(st.Weight-1, engine.Committed(), header.ParentID())
			if err != nil {
				return err
			}

			if err := engine.store.Put(committedKey, id[:]); err != nil {
				return err
			}
			engine.committed.Store(id)
		}

		return nil
	}

	return
}

// MarkVoted marks the voted checkpoint.
// Not thread-safe!
func (engine *BFTEngine) MarkVoted(parentID thor.Bytes32) error {
	checkpoint, err := engine.repo.NewChain(parentID).GetBlockID(getCheckpoint(block.Number(parentID)))
	if err != nil {
		return err
	}

	st, err := engine.getState(parentID, engine.repo.GetBlockHeader)
	if err != nil {
		return err
	}

	engine.voted[checkpoint] = st.Weight
	return nil
}

// GetVote computes the vote for a given parent block ID.
// Not thread-safe!
func (engine *BFTEngine) GetVote(parentID thor.Bytes32) (block.Vote, error) {
	st, err := engine.getState(parentID, engine.repo.GetBlockHeader)
	if err != nil {
		return block.WIT, err
	}

	if st.Weight == 0 {
		return block.WIT, nil
	}

	committed := engine.Committed()

	// most recent justified checkpoint
	var recentJC thor.Bytes32
	var Weight = st.Weight
	if st.JustifyAt != nil {
		// if justied in this round, use this round's checkpoint
		checkpoint, err := engine.repo.NewChain(parentID).GetBlockID(getCheckpoint(block.Number(parentID)))
		if err != nil {
			return block.WIT, err
		}
		recentJC = checkpoint
	} else {
		checkpoint, err := engine.findCheckpointByWeight(Weight, committed, parentID)
		if err != nil {
			return block.WIT, err
		}
		recentJC = checkpoint
	}

	// see https://github.com/vechain/VIPs/blob/master/vips/vip-220.md
	for blockID, w := range engine.voted {
		if block.Number(blockID) >= block.Number(committed) {
			a, b := recentJC, blockID
			if block.Number(blockID) > block.Number(recentJC) {
				a, b = blockID, recentJC
			}

			if includes, err := engine.repo.NewChain(a).HasBlock(b); err != nil {
				return block.WIT, err
			} else if !includes && w >= Weight-1 {
				return block.WIT, nil
			}
		}
	}

	return block.COM, nil
}

// Close closes bft engine.
func (engine *BFTEngine) Close() {
	if len(engine.voted) > 0 {
		toSave := make(map[thor.Bytes32]uint32)
		committed := engine.Committed()

		for k, v := range engine.voted {
			if block.Number(k) >= block.Number(committed) {
				toSave[k] = v
			}
		}

		if len(toSave) > 0 {
			saveVoted(engine.store, toSave)
		}
	}
}

// getState get the bft state regarding the given block id.
func (engine *BFTEngine) getState(blockID thor.Bytes32, getHeader GetBlockHeader) (*bftState, error) {
	if cached, ok := engine.caches.state.Get(blockID); ok {
		return cached.(*bftState), nil
	}

	if block.Number(blockID) == 0 {
		return &bftState{}, nil
	}

	var (
		vs  *voteSet
		end uint32
	)

	header, err := getHeader(blockID)
	if err != nil {
		return nil, err
	}

	if entry := engine.caches.voteset.Remove(header.ParentID()); getCheckpoint(header.Number()) != header.Number() && entry != nil {
		vs = interface{}(entry.Entry.Value).(*voteSet)
		end = block.Number(header.ParentID())
	} else {
		var err error
		vs, err = newVoteSet(engine, header.ParentID())
		if err != nil {
			return nil, errors.Wrap(err, "failed to create vote set")
		}
		end = vs.checkpoint
	}

	h := header
	for {
		if vs.isCommitted() || h.Vote() == nil {
			break
		}

		signer, err := h.Signer()
		if err != nil {
			return nil, err
		}

		vs.addVote(signer, *h.Vote(), h.ID())

		if h.Number() <= end {
			break
		}

		h, err = getHeader(h.ParentID())
		if err != nil {
			return nil, err
		}
	}

	st := vs.getState()

	engine.caches.state.Add(header.ID(), st)
	engine.caches.voteset.Set(header.ID(), vs, float64(header.Number()))
	return st, nil
}

// findCheckpointByWeight finds the first checkpoint reaches the given weight.
func (engine *BFTEngine) findCheckpointByWeight(target uint32, committed, parentID thor.Bytes32) (blockID thor.Bytes32, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
			return
		}
	}()

	searchStart := block.Number(committed)
	if searchStart == 0 {
		searchStart = getCheckpoint(engine.forkConfig.FINALITY)
	}

	c := engine.repo.NewChain(parentID)
	get := func(i int) (uint32, error) {
		id, err := c.GetBlockID(searchStart + uint32(i+1)*thor.CheckpointInterval - 1)
		if err != nil {
			return 0, err
		}
		return engine.getWeight(id)
	}

	n := int((block.Number(parentID) + 1 - searchStart) / thor.CheckpointInterval)
	num := sort.Search(n, func(i int) bool {
		weight, err := get(i)
		if err != nil {
			panic(err)
		}

		return weight >= target
	})

	if num == n {
		return thor.Bytes32{}, errors.New("failed find the block by weight")
	}

	weight, err := get(num)
	if err != nil {
		return thor.Bytes32{}, err
	}

	if weight != target {
		return thor.Bytes32{}, errors.New("failed to find the block by weight")
	}

	return c.GetBlockID(searchStart + uint32(num)*thor.CheckpointInterval)
}

func (engine *BFTEngine) getMaxBlockProposers(sum *chain.BlockSummary) (mbp uint64, err error) {
	if cached, ok := engine.caches.mbp.Get(sum.Header.ID()); ok {
		return cached.(uint64), nil
	}

	defer func() {
		if err != nil {
			engine.caches.mbp.Add(sum.Header.ID(), mbp)
		}
	}()

	state := engine.stater.NewState(sum.Header.StateRoot(), sum.Header.Number(), sum.Conflicts, sum.SteadyNum)
	params, err := builtin.Params.Native(state).Get(thor.KeyMaxBlockProposers)
	if err != nil {
		return
	}
	mbp = params.Uint64()
	if mbp == 0 || mbp > thor.InitialMaxBlockProposers {
		mbp = thor.InitialMaxBlockProposers
	}

	return
}

func (engine *BFTEngine) getWeight(id thor.Bytes32) (weight uint32, err error) {
	if cached, ok := engine.caches.weight.Get(id); ok {
		return cached.(uint32), nil
	}

	defer func() {
		if err != nil {
			engine.caches.weight.Add(id, weight)
		}
	}()

	return loadWeight(engine.store, id)
}

func getCheckpoint(blockNum uint32) uint32 {
	return blockNum / thor.CheckpointInterval * thor.CheckpointInterval
}

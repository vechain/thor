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
	finalizedKey = []byte("bft-finalized")
)

type GetBlockHeader func(id thor.Bytes32) (*block.Header, error)
type commitFunc func() error

// BFTEngine tracks all votes of blocks, computes the finalized checkpoint.
type BFTEngine struct {
	repo       *chain.Repository
	store      kv.Store
	stater     *state.Stater
	forkConfig thor.ForkConfig
	voted      map[thor.Bytes32]uint32
	finalized  atomic.Value
	caches     struct {
		state   *lru.Cache
		quality *lru.Cache
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
	engine.caches.quality, _ = lru.New(1024)
	engine.caches.mbp, _ = lru.New(8)
	engine.caches.voteset = cache.NewPrioCache(16)

	voted, err := loadVoted(engine.store)
	if err != nil {
		return nil, err
	}
	engine.voted = voted

	if val, err := engine.store.Get(finalizedKey); err != nil {
		if !engine.store.IsNotFound(err) {
			return nil, err
		}
		engine.finalized.Store(engine.repo.GenesisBlock().Header().ID())
	} else {
		engine.finalized.Store(thor.BytesToBytes32(val))
	}

	return &engine, nil
}

// Finalized returns the finalized checkpoint.
func (engine *BFTEngine) Finalized() thor.Bytes32 {
	return engine.finalized.Load().(thor.Bytes32)
}

// Accepts checks if the given block is on the same branch of finalized checkpoint.
func (engine *BFTEngine) Accepts(parentID thor.Bytes32) error {
	if block.Number(parentID) < engine.forkConfig.FINALITY {
		return nil
	}

	finalized := engine.Finalized()
	if block.Number(finalized) == 0 {
		return nil
	}

	if included, err := engine.repo.NewChain(parentID).HasBlock(finalized); err != nil {
		return err
	} else if !included {
		return errConflictWithFinalized
	}

	return nil
}

// Process processes block in bft engine and returns whether the block becomes new best.
// Not thread-safe!
func (engine *BFTEngine) Process(header *block.Header) (becomeNewBest bool, commit commitFunc, err error) {
	commit = func() error { return nil }

	best := engine.repo.BestBlockSummary().Header
	if header.Number() < engine.forkConfig.FINALITY || best.Number() < engine.forkConfig.FINALITY {
		becomeNewBest = header.BetterThan(best)
		return
	}

	newSt, err := engine.getState(header.ID(), func(id thor.Bytes32) (*block.Header, error) {
		// header was not added to repo at this moment
		if id == header.ID() {
			return header, nil
		}
		return engine.repo.GetBlockHeader(id)
	})
	if err != nil {
		return
	}

	bestSt, err := engine.getState(best.ID(), engine.repo.GetBlockHeader)
	if err != nil {
		return
	}

	if newSt.Quality != bestSt.Quality {
		becomeNewBest = newSt.Quality > bestSt.Quality
	} else {
		becomeNewBest = header.BetterThan(best)
	}

	commit = func() error {
		// save quality if needed
		if getStorePoint(header.Number()) == header.Number() {
			if err := saveQuality(engine.store, header.ID(), newSt.Quality); err != nil {
				return err
			}
			engine.caches.quality.Add(header.ID(), newSt.Quality)
		}

		// update finalized if new block commits this round
		if newSt.CommitAt != nil && header.ID() == *newSt.CommitAt && newSt.Quality > 1 {
			id, err := engine.findCheckpointByQuality(newSt.Quality-1, engine.Finalized(), header.ParentID())
			if err != nil {
				return err
			}

			if err := engine.store.Put(finalizedKey, id[:]); err != nil {
				return err
			}
			engine.finalized.Store(id)
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

	engine.voted[checkpoint] = st.Quality
	return nil
}

// GetVote computes the vote for a given parent block ID.
// Not thread-safe!
func (engine *BFTEngine) GetVote(parentID thor.Bytes32) (block.Vote, error) {
	st, err := engine.getState(parentID, engine.repo.GetBlockHeader)
	if err != nil {
		return block.WIT, err
	}

	if st.Quality == 0 {
		return block.WIT, nil
	}

	finalized := engine.Finalized()

	// most recent justified checkpoint
	var recentJC thor.Bytes32
	theQuality := st.Quality
	if st.Justified {
		// if justied in this round, use this round's checkpoint
		checkpoint, err := engine.repo.NewChain(parentID).GetBlockID(getCheckpoint(block.Number(parentID)))
		if err != nil {
			return block.WIT, err
		}
		recentJC = checkpoint
	} else {
		checkpoint, err := engine.findCheckpointByQuality(theQuality, finalized, parentID)
		if err != nil {
			return block.WIT, err
		}
		recentJC = checkpoint
	}

	ids := make([]thor.Bytes32, 0, len(engine.voted))
	for k := range engine.voted {
		ids = append(ids, k)
	}
	sort.Slice(ids, func(i, j int) bool {
		return block.Number(ids[i]) > block.Number(ids[j])
	})
	// see https://github.com/vechain/VIPs/blob/master/vips/VIP-220.md
	for _, blockID := range ids {
		if block.Number(blockID) < block.Number(finalized) {
			break
		}

		a, b := recentJC, blockID
		if block.Number(blockID) > block.Number(recentJC) {
			a, b = blockID, recentJC
		}

		votedQuality := engine.voted[blockID]
		if includes, err := engine.repo.NewChain(a).HasBlock(b); err != nil {
			return block.WIT, err
		} else if !includes && votedQuality >= theQuality-1 {
			return block.WIT, nil
		}
	}

	return block.COM, nil
}

// Close closes bft engine.
func (engine *BFTEngine) Close() {
	if len(engine.voted) > 0 {
		toSave := make(map[thor.Bytes32]uint32)
		finalized := engine.Finalized()

		for k, v := range engine.voted {
			if block.Number(k) >= block.Number(finalized) {
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

// findCheckpointByQuality finds the first checkpoint reaches the given quality.
func (engine *BFTEngine) findCheckpointByQuality(target uint32, finalized, parentID thor.Bytes32) (blockID thor.Bytes32, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
			return
		}
	}()

	searchStart := block.Number(finalized)
	if searchStart == 0 {
		searchStart = getCheckpoint(engine.forkConfig.FINALITY)
	}

	c := engine.repo.NewChain(parentID)
	get := func(i int) (uint32, error) {
		id, err := c.GetBlockID(getStorePoint(searchStart + uint32(i)*thor.CheckpointInterval))
		if err != nil {
			return 0, err
		}
		return engine.getQuality(id)
	}

	n := int((block.Number(parentID) + 1 - searchStart) / thor.CheckpointInterval)
	num := sort.Search(n, func(i int) bool {
		quality, err := get(i)
		if err != nil {
			panic(err)
		}

		return quality >= target
	})

	if num == n {
		return thor.Bytes32{}, errors.New("failed find the block by quality")
	}

	quality, err := get(num)
	if err != nil {
		return thor.Bytes32{}, err
	}

	if quality != target {
		return thor.Bytes32{}, errors.New("failed to find the block by quality")
	}

	return c.GetBlockID(searchStart + uint32(num)*thor.CheckpointInterval)
}

func (engine *BFTEngine) getMaxBlockProposers(sum *chain.BlockSummary) (mbp uint64, err error) {
	if cached, ok := engine.caches.mbp.Get(sum.Header.ID()); ok {
		return cached.(uint64), nil
	}

	defer func() {
		if err == nil {
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

func (engine *BFTEngine) getQuality(id thor.Bytes32) (quality uint32, err error) {
	if cached, ok := engine.caches.quality.Get(id); ok {
		return cached.(uint32), nil
	}

	defer func() {
		if err == nil {
			engine.caches.quality.Add(id, quality)
		}
	}()

	return loadQuality(engine.store, id)
}

func getCheckpoint(blockNum uint32) uint32 {
	return blockNum / thor.CheckpointInterval * thor.CheckpointInterval
}

// save quality at the end of round
func getStorePoint(blockNum uint32) uint32 {
	return getCheckpoint(blockNum) + thor.CheckpointInterval - 1
}
